package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/mcubik/goverreport/report"
	"github.com/olekukonko/tablewriter"
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
	cmd := exec.Command("go", "test", "-json", "./...", "-covermode=atomic", "-coverprofile", "/tmp/coverage.out")
	var errOut bytes.Buffer
	var out bytes.Buffer
	var events []*event
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
	rep, err := report.GenerateReport("/tmp/coverage.out", "", nil, "filename", "asc", false)
	if err != nil {
		log.Panic(err)
	}
	report.PrintTable(rep, os.Stdout, false)
	PrintTable(events, os.Stdout)
}

func PrintTable(events []*event, writer io.Writer) {
	table := tablewriter.NewWriter(writer)

	table.SetColumnAlignment([]int{
		tablewriter.ALIGN_LEFT,
		tablewriter.ALIGN_RIGHT,
		tablewriter.ALIGN_RIGHT,
		tablewriter.ALIGN_RIGHT,
		tablewriter.ALIGN_RIGHT,
	})
	table.SetFooterAlignment(tablewriter.ALIGN_LEFT)
	table.SetHeader([]string{
		"Test",
		"Result",
		"Package",
	})
	successCount := 0
	failCount := 0
	total := 0
	for _, event := range events {
		if event.Action == "pass" || event.Action == "fail" {
			if event.Test == "" {
				continue
			}
			total += 1
			if event.Action == "pass" {
				successCount += 1
			} else {
				failCount += 1
			}
			table.Append([]string{
				event.Test,
				event.Action,
				event.Package,
			})
		}
	}
	table.SetFooter([]string{
		fmt.Sprintf("total %d", total),
		fmt.Sprintf("pass %d,fail %d", successCount, failCount),
		"",
	})
	table.SetAutoFormatHeaders(false)
	table.Render()
}
