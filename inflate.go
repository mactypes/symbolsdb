package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/qri-io/jsonpointer"
)

type Symbol struct {
	Name string
	Path string
	Kind string

	Description  string            // /abstract/$content
	Type         string            // /metadata/roleHeading
	Parent       string            // /metadata/parent/title
	Modules      []string          // /metadata/modules
	Platforms    []Platform        // /metadata/platforms
	Deprecated   bool              // /deprecationSummary
	Declaration  string            // if there is only one declaration, its put here. otherwise under Declarations[platform]
	Declarations map[string]string // /primaryContentSections/[kind=declarations]/declarations/[languages=[occ]/tokens (key is platform)
	Parameters   []Parameter       // /primaryContentSections/[kind=parameters]/parameters (name:/name,description:/content/0/inlineContent/$content)
	Return       string            // /primaryContentSections/?[kind=content]/content/0/anchor=return_value ../1/inlineContent/$content
	InheritsFrom string            // /relationshipSections/[type=inheritsFrom]/identifiers/0
}

type Platform struct {
	Name         string
	IntroducedAt string
	Current      string
	Beta         bool
	Deprecated   bool
	DeprecatedAt string
}

type Parameter struct {
	Name        string
	Description string
}

var known404 []string

func main() {
	var err error
	known404, err = readFileLines("./404")
	if err != nil {
		log.Fatal(err)
	}

	if len(os.Args) > 1 {
		spew.Dump(inflate(fmt.Sprintf("./symbols/%s.json", os.Args[1])))
		return
	}

	err = filepath.Walk("./symbols", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && filepath.Ext(path) == ".json" {
			//fmt.Println(path)
			s := inflate(path)
			b, err := json.MarshalIndent(s, "", "  ")
			if err != nil {
				log.Fatal(err)
			}
			if err := ioutil.WriteFile(path, b, 0644); err != nil {
				log.Fatal(err)
			}
		}

		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
}

func inflate(symbolPath string) Symbol {
	metaPath := strings.Replace(symbolPath, "symbols", "cache/meta", 1)

	sym, err := loadData[Symbol](symbolPath)
	if err != nil {
		log.Fatal(err, " ", symbolPath)
	}

	if strIn(known404, sym.Path) {
		return sym
	}

	doc, err := loadData[map[string]interface{}](metaPath)
	if err != nil {
		log.Fatal(err, " ", metaPath)
	}

	fmt.Println(metaPath)

	// fix bug in constant names
	if sym.Kind == "Constant" {
		sym.Name = strings.Split(sym.Name, " = ")[0]
	}

	// Description
	if abstract := findPath(doc, "/abstract"); abstract != nil {
		sym.Description = strings.Trim(parseContent(abstract), " ")
	}
	// Type
	if typ := findPath(doc, "/metadata/roleHeading"); typ != nil {
		sym.Type = typ.(string)
	}
	// Platforms
	sym.Platforms = parsePlatforms(findPath(doc, "/metadata/platforms"))
	// Modules
	if mods := findPath(doc, "/metadata/modules"); mods != nil {
		sym.Modules = []string{}
		for _, m := range mods.([]any) {
			sym.Modules = append(sym.Modules, findPath(m, "/name").(string))
		}
	}
	// Parent
	if parent := findPath(doc, "/metadata/parent/title"); parent != nil {
		sym.Parent = parent.(string)
	}

	if content := findPath(doc, "/primaryContentSections"); content != nil {
		// Parameters
		if paramContent := findWithProp(content, "kind", "parameters"); paramContent != nil {
			if params := findPath(paramContent, "/parameters"); params != nil {
				sym.Parameters = []Parameter{}
				for _, param := range params.([]any) {
					sym.Parameters = append(sym.Parameters, Parameter{
						Name:        findPath(param, "/name").(string),
						Description: parseContent(findPath(param, "/content/0/inlineContent")),
					})
				}
			}
		}
		// Return
		for _, potentialRet := range content.([]any) {
			if anchor := findPath(potentialRet, "/content/0/anchor"); anchor != nil && anchor.(string) == "return_value" {
				sym.Return = parseContent(findPath(potentialRet, "/content/1/inlineContent"))
			}
		}
		// Declaration
		sym.Declarations = make(map[string]string)
		if declContent := findWithProp(content, "kind", "declarations"); declContent != nil {
			if decls := findPath(declContent, "/declarations"); decls != nil {
				for _, decl := range decls.([]any) {
					langs := findPath(decl, "/languages")
					if langs == nil {
						continue
					}
					found := false
					for _, lang := range langs.([]any) {
						if lang.(string) == "occ" {
							found = true
						}
					}
					if !found {
						continue
					}
					declStr := buildDeclarationFromTokens(findPath(decl, "/tokens"))
					platforms := findPath(decl, "/platforms")
					if platforms == nil {
						panic("platforms not found for declaration on " + sym.Path)
					}
					for _, platform := range platforms.([]any) {
						platName := strings.ToLower(platform.(string))
						sym.Declarations[platName] = declStr
					}
				}
			}
		}
		// if all Declarations are the same, unset Declarations and set Declaration
		decl := ""
		same := true
		for _, d := range sym.Declarations {
			if decl == "" {
				decl = d
			} else {
				if decl != d {
					same = false
					break
				}
			}
		}
		if same {
			sym.Declarations = nil
			sym.Declaration = decl
		}
	}

	// Deprecated
	if findPath(doc, "/deprecationSummary") != nil {
		sym.Deprecated = true
	}

	// InheritsFrom
	if sections := findPath(doc, "/relationshipsSections"); sections != nil {
		if inheritsFrom := findWithProp(sections, "type", "inheritsFrom"); inheritsFrom != nil {
			if id := findPath(inheritsFrom, "/identifiers/0"); id != nil {
				sym.InheritsFrom = strings.Replace(id.(string), "doc://com.apple.documentation/documentation/", "", 1)
			}
		}
	}

	// sanity check declaration, unless any of these cases...
	ignoreDeclaration := false
	if role := findPath(doc, "/metadata/role"); role != nil &&
		(role.(string) == "collectionGroup" ||
			role.(string) == "dictionarySymbol") {
		ignoreDeclaration = true
	}
	if strings.HasPrefix(sym.Path, "kernel") {
		ignoreDeclaration = true
	}
	if lang := findPath(doc, "/identifier/interfaceLanguage"); lang != nil {
		if lang.(string) == "swift" {
			ignoreDeclaration = true
		}
	}
	if sym.Kind != "Framework" && sym.Declaration == "" && len(sym.Declarations) == 0 && !sym.Deprecated && sym.Type != "" && !ignoreDeclaration {
		log.Fatal("no declaration ", sym.Path, " ", sym.Kind)
	}

	return sym
}

func loadData[T any](filepath string) (v T, err error) {
	b, err := ioutil.ReadFile(filepath)
	if err != nil {
		return
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return v, err
	}
	return
}

func parseContent(content any) string {
	if content == nil {
		return ""
	}
	str := ""
	for _, part := range content.([]any) {
		typ := findPath(part, "/type")
		if typ == nil {
			continue
		}
		switch typ {
		case "text":
			if text := findPath(part, "/text"); text != nil {
				str += text.(string)
			}
		case "codeVoice":
			if code := findPath(part, "/code"); code != nil {
				str += code.(string)
			}
		case "inlineHead":
			str += parseContent(findPath(part, "/inlineContent")) + ": "
		case "emphasis", "strong", "newTerm", "superscript":
			str += parseContent(findPath(part, "/inlineContent"))
		case "reference":
			if id := findPath(part, "/identifier"); id != nil {
				str += resolveRefName(id.(string))
			}
		default:
			log.Fatal("unknown content part type: ", typ)
		}
	}
	return str
}

func resolveRefName(identifier string) string {
	path := strings.Replace(identifier, "doc://com.apple.documentation/documentation/", "", 1)
	parts := strings.Split(path, "/")
	for idx, part := range parts {
		if idx == 0 {
			continue
		}
		dash := strings.LastIndex(part, "-")
		if dash > 0 && dash < len(part)-1 {
			parts[idx] = part[dash+1:]
		}
	}
	symbolfile := fmt.Sprintf("./symbols/%s.json", filepath.Join(parts...))
	s, err := loadData[Symbol](symbolfile)
	if err != nil {
		return fmt.Sprintf("[%s]", filepath.Join(parts...))
	}
	return s.Name
}

func parsePlatforms(platforms any) (plats []Platform) {
	if platforms == nil {
		return nil
	}
	for _, p := range platforms.([]any) {
		current := ""
		if cur := findPath(p, "/current"); cur != nil {
			current = cur.(string)
		}
		pp := Platform{
			Name:         findPath(p, "/name").(string),
			IntroducedAt: findPath(p, "/introducedAt").(string),
			Current:      current,
		}
		if beta := findPath(p, "/beta"); beta != nil {
			pp.Beta = beta.(bool)
		}
		if deprecated := findPath(p, "/deprecated"); deprecated != nil {
			pp.Deprecated = deprecated.(bool)
			deprecatedAt := ""
			if dep := findPath(p, "/deprecatedAt"); dep != nil {
				deprecatedAt = dep.(string)
			}
			pp.DeprecatedAt = deprecatedAt
		}
		plats = append(plats, pp)
	}
	return
}

func buildDeclarationFromTokens(tokens any) string {
	if tokens == nil {
		return ""
	}
	str := ""
	for _, t := range tokens.([]any) {
		str += t.(map[string]any)["text"].(string)
	}
	return str
}

func findPath(doc any, ptr string) any {
	p, err := jsonpointer.Parse(ptr)
	if err != nil {
		panic(err)
	}
	v, err := p.Eval(doc)
	if err != nil {
		if strings.Contains(err.Error(), "invalid JSON pointer") {
			return nil
		}
		if strings.Contains(err.Error(), "exceeds array length") {
			return nil
		}
		panic(err)
	}
	return v
}

func findWithProp(doc any, key string, value any) any {
	if doc == nil {
		return nil
	}
	for _, obj := range doc.([]any) {
		v := findPath(obj, "/"+key)
		if v != nil && reflect.DeepEqual(v, value) {
			return obj
		}
	}
	return nil
}

func strIn(slice []string, str string) bool {
	for _, s := range slice {
		if strings.HasPrefix(str, s) {
			return true
		}
	}
	return false
}

func readFileLines(filename string) ([]string, error) {
	var lines []string
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}
