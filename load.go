package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

type Symbol struct {
	Name string
	Path string
	Kind string
}

func main() {
	targetDir := "./symbols"
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		log.Fatal(err)
	}

	// Open the docSet database
	db, err := sql.Open("sqlite3", "./cache/docSet.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	err = db.Ping()
	if err != nil {
		log.Fatal(err)
	}

	rows, err := db.Query("SELECT * FROM searchIndex WHERE type NOT IN ('Guide', 'Request', 'Object', 'Sample') AND path LIKE 'dash-apple-api://load?request_key=lc%'")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	symbols := make(map[string][]Symbol)

	for rows.Next() {
		var id int
		var name string
		var kind string
		var path string

		err = rows.Scan(&id, &name, &kind, &path)
		if err != nil {
			log.Fatal(err)
		}

		hash := strings.Index(path, "#")
		path = strings.TrimPrefix(path[:hash], "dash-apple-api://load?request_key=lc/documentation/")

		if symbols[kind] == nil {
			symbols[kind] = make([]Symbol, 0)
		}
		symbols[kind] = append(symbols[kind], Symbol{
			Name: name,
			Kind: kind,
			Path: path,
		})

	}

	err = rows.Err()
	if err != nil {
		log.Fatal(err)
	}

	loaded := 0

	fmt.Println("Loading frameworks...")
	for _, s := range symbols["Framework"] {
		fwfile := filepath.Join(targetDir, fmt.Sprintf("%s.json", s.Path))
		if err := writeJSON(fwfile, s); err != nil {
			log.Fatal(err)
		}
		loaded++
		if err := os.MkdirAll(filepath.Join(targetDir, s.Path), 0755); err != nil {
			log.Fatal(err)
		}
	}

	fmt.Println("Loading classes...")
	for _, s := range symbols["Class"] {
		clsfile := filepath.Join(targetDir, fmt.Sprintf("%s.json", s.Path))

		if err := os.MkdirAll(filepath.Dir(clsfile), 0755); err != nil {
			log.Fatal(err)
		}
		if err := writeJSON(clsfile, s); err != nil {
			log.Fatal(err)
		}
		loaded++
	}

	fmt.Println("Loading protocols...")
	for _, s := range symbols["Protocol"] {
		protofile := filepath.Join(targetDir, fmt.Sprintf("%s.json", s.Path))
		if _, err := os.Stat(protofile); err == nil {
			// class exists with this name, so
			// for now we skip it. the methods for
			// the protocol will end up under the class.
			continue
		}

		if err := writeJSON(protofile, s); err != nil {
			log.Fatal(err)
		}
		loaded++
	}

	fmt.Println("Loading methods...")
	for _, s := range symbols["Method"] {
		if strings.Contains(s.Path, "java_support") ||
			strings.Contains(s.Path, "deprecated_symbols") ||
			strings.Contains(s.Path, "objective-c_runtime") ||
			strIn([]string{
				"kernel/1441813-getaddress",
				"kernel/1441811-getsize",
				"kernel/1534574-getkey",
				"kernel/1547721-weakwithspecification",
			}, s.Path) { // weird struct methods
			continue
		}
		methodfile := filepath.Join(targetDir, fmt.Sprintf("%s.json", s.Path))

		if err := os.MkdirAll(filepath.Dir(methodfile), 0755); err != nil {
			log.Fatal(err)
		}
		if err := writeJSON(methodfile, s); err != nil {
			log.Fatal(err)
		}
		loaded++
	}

	fmt.Println("Loading structs...")
	files := map[string]string{}
	for _, s := range symbols["Struct"] {
		if strings.Contains(s.Path, "_h/") || // unnecessary low level api collection
			strings.Contains(s.Path, "_h_") || // unnecessary low level api collection
			strings.Contains(s.Path, "java_support") ||
			strings.Contains(s.Path, "deprecated_symbols") ||
			strings.Contains(s.Path, "objective-c_runtime") ||
			strIn([]string{
				"applicationservices/core_printing/pmresolution",
				"applicationservices/core_printing/pmrect",
				"applicationservices/core_printing/pmlanguageinfo",
				"coreservices/carbon_core/core_endian/bigendianostype",
			}, s.Path) || // avoid conflicts by skipping these
			strings.Contains(s.Name, "::") { // odd namespaced types, not many
			continue
		}

		structfile := filepath.Join(targetDir, fmt.Sprintf("%s.json", s.Path))
		if p, exists := files[structfile]; exists {
			fmt.Println("CONFLICT:", s.Path, p)
		}

		if err := os.MkdirAll(filepath.Dir(structfile), 0755); err != nil {
			log.Fatal(err)
		}
		if err := writeJSON(structfile, s); err != nil {
			log.Fatal(err)
		}
		loaded++
		files[structfile] = s.Path
	}

	fmt.Println("Loading properties...")
	for _, s := range symbols["Property"] {
		if strings.Contains(s.Path, "java_support") ||
			strings.Contains(s.Path, "deprecated_symbols") ||
			strings.Contains(s.Path, "objective-c_runtime") ||
			len(strings.Split(s.Path, "/")) > 3 ||
			strIn([]string{
				"bundleresources/entitlements",
				"bundleresources/information_property_list",
			}, s.Path) { // not props, proplists ugh
			continue
		}
		propfile := filepath.Join(targetDir, fmt.Sprintf("%s.json", s.Path))

		if err := os.MkdirAll(filepath.Dir(propfile), 0755); err != nil {
			log.Fatal(err)
		}
		if err := writeJSON(propfile, s); err != nil {
			log.Fatal(err)
		}
		loaded++
	}

	fmt.Println("Loading unions...")
	files = map[string]string{}
	for _, s := range symbols["Union"] {
		if strings.Contains(s.Path, "_h/") || // unnecessary low level api collection
			strings.Contains(s.Path, "_h_") || // unnecessary low level api collection
			strings.Contains(s.Path, "java_support") ||
			strings.Contains(s.Path, "deprecated_symbols") ||
			strings.Contains(s.Path, "objective-c_runtime") {
			continue
		}

		unionfile := filepath.Join(targetDir, fmt.Sprintf("%s.json", s.Path))
		if p, exists := files[unionfile]; exists {
			fmt.Println("CONFLICT:", s.Path, p)
		}

		if err := os.MkdirAll(filepath.Dir(unionfile), 0755); err != nil {
			log.Fatal(err)
		}
		if err := writeJSON(unionfile, s); err != nil {
			log.Fatal(err)
		}
		loaded++
		files[unionfile] = s.Path
	}

	fmt.Println("Loading types...")
	files = map[string]string{}
	for _, s := range symbols["Type"] {
		if strings.Contains(s.Path, "_h/") || // unnecessary low level api collection
			strings.Contains(s.Path, "_h_") || // unnecessary low level api collection
			strings.Contains(s.Path, "java_support") ||
			strings.Contains(s.Path, "deprecated_symbols") ||
			strings.Contains(s.Path, "objective-c_runtime") ||
			strings.Contains(s.Path, "/entitlements/") ||
			strings.Contains(s.Path, "bundleresources/information_property_list") ||
			strings.Contains(s.Name, " ") || // usually plist type
			strIn([]string{
				"opendirectory/opendirectory_functions/odauthenticationtype",
				"opendirectory/opendirectory_functions/odattributetype",
				"opendirectory/opendirectory_functions/odrecordtype",
			}, s.Path) || // avoid conflicts by skipping these
			strings.Contains(s.Name, "::") { // odd namespaced types, not many
			continue
		}

		typefile := filepath.Join(targetDir, fmt.Sprintf("%s.json", s.Path))
		if p, exists := files[typefile]; exists {
			fmt.Println("CONFLICT:", s.Path, p)
		}

		if err := os.MkdirAll(filepath.Dir(typefile), 0755); err != nil {
			log.Fatal(err)
		}
		if err := writeJSON(typefile, s); err != nil {
			log.Fatal(err)
		}
		loaded++
		files[typefile] = s.Path
	}

	fmt.Println("Loading enums...")
	files = map[string]string{}
	for _, s := range symbols["Enum"] {
		if strings.Contains(s.Path, "_h/") || // unnecessary low level api collection
			strings.Contains(s.Path, "_h_") || // unnecessary low level api collection
			strings.Contains(s.Path, "java_support") ||
			strings.Contains(s.Path, "deprecated_symbols") ||
			strings.Contains(s.Path, "objective-c_runtime") ||
			strIn([]string{
				"iokit/1503935-control",
				"iokit/1503882-control",
				"professional_video_applications/3656031-fxanalysisstate",
				"audiotoolbox/auaudiounit/auaudiounitbustype",
				"audiotoolbox/auaudiounit/auhosttransportstateflags",
				"audiotoolbox/auaudiounit/aurendereventtype",
			}, s.Path) || // avoid conflicts by skipping these
			strings.Contains(s.Name, " ") { // usually plist property
			continue
		}

		enumfile := filepath.Join(targetDir, fmt.Sprintf("%s.json", s.Path))
		if p, exists := files[enumfile]; exists {
			fmt.Println("CONFLICT:", s.Path, p)
		}

		if err := os.MkdirAll(filepath.Dir(enumfile), 0755); err != nil {
			log.Fatal(err)
		}
		if err := writeJSON(enumfile, s); err != nil {
			log.Fatal(err)
		}
		loaded++
		files[enumfile] = s.Path
	}

	fmt.Println("Loading constants...")
	files = map[string]string{}
	for _, s := range symbols["Constant"] {
		if strIn([]string{
			"foundation/nsmaptableoptions/nsmaptablezeroingweakmemory",
			"foundation/nsmaptableoptions/nsmaptablestrongmemory",
			"foundation/nsmaptableoptions/nsmaptablecopyin",
			"foundation/nsmaptableoptions/nsmaptableweakmemory",
			"foundation/nsmaptableoptions/nsmaptableobjectpointerpersonality",
			"coremidi/midiobjecttype/kmidiobjecttype_externalmask",
			"addressbook/address_book_constants/error_codes/abpropertyvaluevalidationerror",
			"addressbook/address_book_constants/error_codes/abpropertyunsupportedbysourceerror",
			"addressbook/address_book_constants/error_codes/abpropertyreadonlyerror",
			"addressbook/address_book_constants/error_codes/abremoverecordserror",
			"addressbook/address_book_constants/error_codes/abaddrecordserror",
			"opendirectory/opendirectory_functions/match_types/kodmatchinsensitivebeginswith",
			"opendirectory/opendirectory_functions/match_types/kodmatchinsensitiveequalto",
			"opendirectory/opendirectory_functions/match_types/kodmatchinsensitiveendswith",
			"opendirectory/opendirectory_functions/match_types/kodmatchinsensitivecontains",
			"opendirectory/opendirectory_functions/match_types/kodmatchbeginswith",
			"opendirectory/opendirectory_functions/match_types/kodmatchequalto",
			"opendirectory/opendirectory_functions/match_types/kodmatchendswith",
			"opendirectory/opendirectory_functions/match_types/kodmatchcontains",
			"opendirectory/opendirectory_functions/match_types/kodmatchgreaterthan",
			"opendirectory/opendirectory_functions/match_types/kodmatchlessthan",
			"opendirectory/opendirectory_functions/match_types/kodmatchany",
			"appkit/nsstackviewvisibilitypriority/nsstackviewvisibilityprioritydetachonlyifnecessary",
			"appkit/nsstackviewvisibilitypriority/nsstackviewvisibilityprioritymusthold",
			"appkit/nsstackviewvisibilitypriority/nsstackviewvisibilityprioritynotvisible",
			"coretext/ctfontdescriptor/font_class_mask_shift_constants/kctfontclassmaskshift",
			"coreaudiotypes/coreaudiotype_constants/kaudiostreamanyrate/kaudiostreamanyrate",
			"applicationservices/axvaluetype/kaxvalueaxerrortype",
			"applicationservices/axvaluetype/kaxvaluecfrangetype",
			"applicationservices/axvaluetype/kaxvaluecgpointtype",
			"applicationservices/axvaluetype/kaxvalueillegaltype",
			"applicationservices/axvaluetype/kaxvaluecgrecttype",
			"applicationservices/axvaluetype/kaxvaluecgsizetype",
			"corefoundation/base_utilities/value_not_found/kcfnotfound",
			"coregraphics/cgfont/font_table_index_values/kcgfontindexinvalid",
			"coregraphics/cgfont/font_table_index_values/kcgfontindexmax",
			"coregraphics/cgfont/font_table_index_values/kcgglyphmax",
		}, s.Path) || // avoid conflicts by skipping these
			strings.Contains(s.Path, "_h/") || // unnecessary low level api collection
			strings.Contains(s.Path, "_h_") || // unnecessary low level api collection
			strings.Contains(s.Path, "java_support") ||
			strings.Contains(s.Path, "deprecated") ||
			strings.Contains(s.Path, "objective-c_runtime") {
			continue
		}

		s.Name = strings.Split(s.Name, " = ")[0]
		constfile := filepath.Join(targetDir, fmt.Sprintf("%s.json", s.Path))
		if p, exists := files[constfile]; exists {
			fmt.Println("CONFLICT:", s.Path, p)
		}

		if err := os.MkdirAll(filepath.Dir(constfile), 0755); err != nil {
			log.Fatal(err)
		}
		if err := writeJSON(constfile, s); err != nil {
			log.Fatal(err)
		}
		loaded++
		files[constfile] = s.Path
	}

	fmt.Println("Loading macros...")
	files = map[string]string{}
	for _, s := range symbols["Macro"] {
		if strings.Contains(s.Path, "_h/") || // unnecessary low level api collection
			strings.Contains(s.Path, "_h_") || // unnecessary low level api collection
			strings.Contains(s.Path, "java_support") ||
			strings.Contains(s.Path, "deprecated_symbols") ||
			strings.Contains(s.Path, "objective-c_runtime") ||
			strIn([]string{
				"applicationservices/core_printing/pdf_workflow_dictionary_keys/kpdfworkflowitemurlkey",
			}, s.Path) || // avoid conflicts by skipping these
			s.Name == "kFxPropertyKey_EquivalentSMPTEWipeCode" || // suspicious, no doc page
			s.Name == "Constant" { // suspicious, no doc page
			continue
		}

		macrofile := filepath.Join(targetDir, fmt.Sprintf("%s.json", s.Path))
		if p, exists := files[macrofile]; exists {
			fmt.Println("CONFLICT:", s.Path, p)
		}

		if err := os.MkdirAll(filepath.Dir(macrofile), 0755); err != nil {
			log.Fatal(err)
		}
		if err := writeJSON(macrofile, s); err != nil {
			log.Fatal(err)
		}
		loaded++
		files[macrofile] = s.Path
	}

	fmt.Println("Loading functions...")
	files = map[string]string{}
	for _, s := range symbols["Function"] {
		if strings.Contains(s.Path, "_h/") ||
			strings.Contains(s.Path, "_h_") ||
			strings.Contains(s.Path, "java_support") ||
			strings.Contains(s.Path, "deprecated_symbols") ||
			strings.Contains(s.Path, "objective-c_runtime") ||
			strings.Contains(s.Name, "::") { // odd namespaced functions, not many
			continue
		}
		fnfile := filepath.Join(targetDir, fmt.Sprintf("%s.json", s.Path))

		// don't care about conflicts because there's too many.
		// look like overloaded functions, but also not important ones.
		// we'll just be ok using the last one...
		//
		// if p, exists := files[fnfile]; exists {
		// 	fmt.Println("CONFLICT:", s.Path, p)
		// }

		if err := os.MkdirAll(filepath.Dir(fnfile), 0755); err != nil {
			log.Fatal(err)
		}

		if err := writeJSON(fnfile, s); err != nil {
			log.Fatal(err)
		}
		loaded++
		files[fnfile] = s.Path
	}

	fmt.Printf("\nLoaded %d symbols.\n", loaded)

}

func strIn(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}

func writeJSON(filepath string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := ioutil.WriteFile(filepath, b, 0644); err != nil {
		return err
	}
	return nil
}
