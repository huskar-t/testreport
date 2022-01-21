package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mcubik/goverreport/report"

	"github.com/olekukonko/tablewriter"
	"golang.org/x/mod/modfile"
)

//	run    - the test has started running
//	pause  - the test has been paused
//	cont   - the test has continued running
//	pass   - the test passed
//	bench  - the benchmark printed log output but did not fail
//	fail   - the test or benchmark failed
//	output - the test printed output
//	skip   - the test was skipped or the package contained no tests

type event struct {
	Time    *time.Time
	Action  string
	Package string
	Test    string
	Elapsed *float64
	Output  string
}

func main() {
	outType := flag.String("t", "a", "a(all) c(coverage) t (test) v(vet)")
	output := flag.String("o", "", "output file default stdout")
	flag.Parse()
	var f io.WriteCloser = os.Stdout
	defer f.Close()
	if output != nil && len(*output) != 0 {
		file, err := os.Create(*output)
		if err != nil {
			panic(err)
		}
		f = file
	}

	switch *outType {
	case "c", "t":
		tmpFilePath, events := doTest()
		if *output == "c" {
			rep, err := report.GenerateReport(tmpFilePath, "", nil, "filename", "asc", false)
			if err != nil {
				log.Panic(err)
			}
			report.PrintTable(rep, f, false)
		} else {
			infos := GetDescription()
			m := PrepareMessage(events, infos)
			PrintTable(m, f)
		}
	case "v":
		vet(f)
	case "a":
		tmpFilePath, events := doTest()
		rep, err := report.GenerateReport(tmpFilePath, "", nil, "filename", "asc", false)
		if err != nil {
			log.Panic(err)
		}
		report.PrintTable(rep, f, false)
		infos := GetDescription()
		m := PrepareMessage(events, infos)
		PrintTable(m, f)
		vet(f)
	}
}

type Message struct {
	Data         [][]string
	Total        int
	SuccessCount int
	FailCount    int
}

func PrepareMessage(events []*event, infos map[string]map[string]*TestInfo) *Message {
	m := &Message{}
	es := map[string][]*event{}
	for _, e := range events {
		if len(e.Test) > 0 && (e.Action == "pass" || e.Action == "fail") {
			es[e.Package] = append(es[e.Package], e)
			m.Total += 1
			if e.Action == "pass" {
				m.SuccessCount += 1
			} else {
				m.FailCount += 1
			}
		}
	}
	for packageName, testList := range es {
		sort.Slice(testList, func(i, j int) bool {
			return testList[i].Test < testList[j].Test
		})
		for _, e := range testList {
			tests, exist := infos[packageName]
			author := ""
			description := ""
			t1 := ""
			t2 := ""
			ts := strings.Split(e.Test, "/")
			switch len(ts) {
			case 1:
				t1 = ts[0]
			case 2:
				t1 = ts[0]
				t2 = ts[1]
			default:
				panic(ts)
			}
			if exist {
				desc, exist := tests[t1]
				if exist {
					description = desc.Description
					author = desc.Author
					if len(t2) != 0 {
						description = fmt.Sprintf("%s(%s)", description, t2)
					}
				}
			}
			m.Data = append(m.Data, []string{
				e.Package,
				t1,
				t2,
				e.Action,
				description,
				author,
			})
		}
	}
	return m
}
func PrintTable(m *Message, writer io.Writer) {
	table := tablewriter.NewWriter(writer)
	table.SetRowLine(true)
	table.SetAutoMergeCellsByColumnIndex([]int{0, 1})
	table.SetColumnAlignment([]int{
		tablewriter.ALIGN_DEFAULT,
		tablewriter.ALIGN_DEFAULT,
		tablewriter.ALIGN_DEFAULT,
		tablewriter.ALIGN_DEFAULT,
		tablewriter.ALIGN_DEFAULT,
		tablewriter.ALIGN_DEFAULT,
	})

	table.SetHeader([]string{
		"Package",
		"Test",
		"SubTest",
		"Result",
		"Description",
		"Author",
	})
	table.AppendBulk(m.Data)
	table.SetFooter([]string{
		"",
		"",
		fmt.Sprintf("total %d", m.Total),
		fmt.Sprintf("pass %d,fail %d", m.SuccessCount, m.FailCount),
		"",
		"",
	})
	table.Render()
}

type TestInfo struct {
	Time        time.Time
	Name        string
	Description string
	Author      string
	PackageName string
}

const LAYOUT = "2006/1/2 15:04"

func GetDescription() map[string]map[string]*TestInfo {
	currentDir := filepath.Dir(os.Args[0])
	data, err := ioutil.ReadFile(path.Join(currentDir, "go.mod"))
	if err != nil {
		panic(err)
	}
	mod, err := modfile.Parse("tmp", data, nil)
	if err != nil {
		panic(err)
	}
	modName := mod.Module.Mod.String()
	testAll := map[string]map[string]*TestInfo{}
	filepath.Walk(currentDir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !isTestFile(path) {
			return nil
		}
		infos := do(path)
		relPath, err := filepath.Rel(currentDir, path)
		if err != nil {
			panic(err)
		}
		relPath = filepath.ToSlash(filepath.Dir(relPath))
		for _, testInfo := range infos {
			modPath := modName
			if len(relPath) > 0 {
				modPath = modPath + "/" + relPath
			}
			_, exist := testAll[modPath]
			if !exist {
				testAll[modPath] = map[string]*TestInfo{
					testInfo.Name: testInfo,
				}
			} else {
				testAll[modPath][testInfo.Name] = testInfo
			}
		}
		return nil
	})
	return testAll
}

func isTestFile(file string) bool {
	return strings.HasSuffix(file, "_test.go")
}

func do(file string) []*TestInfo {
	var infos []*TestInfo
	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, file, nil, parser.ParseComments)
	packageName := astFile.Name.String()
	for _, decl := range astFile.Decls {
		funcDecl, is := decl.(*ast.FuncDecl)
		if is {
			if token.IsExported(funcDecl.Name.Name) {
				doc := funcDecl.Doc.Text()
				info := parseComment(doc)
				if info != nil {
					info.Name = funcDecl.Name.String()
					info.PackageName = packageName
					infos = append(infos, info)
				}
			}
		}
	}
	if err != nil {
		log.Panicf("parse file %s fail :%s", file, err.Error())
	}
	return infos
}

func parseComment(comment string) *TestInfo {
	s := strings.Split(comment, "\n")
	if len(s) == 0 {
		return nil
	}
	m := map[string]string{}

	for _, ss := range s {
		ss = strings.TrimSpace(ss)
		if len(ss) == 0 {
			continue
		}
		kv := strings.SplitN(ss, ":", 2)
		if len(kv) != 2 {
			continue
		}
		k, v := strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1])
		m[k] = v
	}
	if len(m) == 0 {
		return nil
	}
	var err error
	info := &TestInfo{}
	if t, ok := m["@date"]; ok {
		info.Time, err = time.Parse(LAYOUT, t)
		if err != nil {
			log.Panicf("parse @date error:%s", err.Error())
		}
	}
	if name, ok := m["@name"]; ok {
		info.Name = name
	}
	if description, ok := m["@description"]; ok {
		info.Description = description
	}
	if author, ok := m["@author"]; ok {
		info.Author = author
	}
	return info
}

func vet(f io.Writer) {
	cmd := exec.Command("go", "vet", "-json", "./...")
	var errOut bytes.Buffer
	var out bytes.Buffer

	cmd.Stdout = &out
	cmd.Stderr = &errOut
	err := cmd.Start()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Waiting for command to finish...")
	fmt.Println(cmd.Args)
	err = cmd.Wait()
	if err != nil {
		log.Printf("Command finished with error: %v", err)
	}
	if errOut.Len() != 0 {
		f.Write(errOut.Bytes())
	} else {
		f.Write(out.Bytes())
	}
}

func doTest() (string, []*event) {
	tmpFile, err := ioutil.TempFile("", "coverage.out")
	if err != nil {
		panic(err)
	}
	tmpFilePath, err := filepath.Abs(tmpFile.Name())
	if err != nil {
		panic(err)
	}
	cmd := exec.Command("go", "test", "-json", "./...", "-covermode=atomic", "-coverprofile", tmpFilePath)
	var errOut bytes.Buffer
	var out bytes.Buffer
	var events []*event
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	err = cmd.Start()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Waiting for command to finish...")
	fmt.Println(cmd.Args)
	err = cmd.Wait()
	if err != nil {
		log.Printf("Command finished with error: %v", err)
	}
	if errOut.Len() != 0 {
		log.Fatalf("Error out put: %s", errOut.String())
	}
	for {
		line, err := out.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Panic(err)
		}
		var data event
		err = json.Unmarshal(line, &data)
		if err != nil {
			log.Panic(err)
		}
		events = append(events, &data)
	}
	return tmpFilePath, events
}
