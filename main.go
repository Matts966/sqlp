package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"

	"cloud.google.com/go/bigquery"
	"github.com/atotto/clipboard"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"golang.org/x/net/context"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/iterator"
)

type valueListWithSchema struct {
	vals   []bigquery.Value
	schema bigquery.Schema
}

var pages [][][]string
var currentPage = 0

const PAGE_SIZE = 30

func drawPage(table *tview.Table, page int) {
	table.Clear()
	for r, row := range pages[page] {
		color := tcell.ColorWhite
		if r == 0 {
			color = tcell.ColorTeal
		}
		for c, cell := range row {
			table.SetCell(r+1, c,
				tview.NewTableCell(cell).
					SetTextColor(color).
					SetAlign(tview.AlignLeft))
		}
	}
}

func drawFrame(frame *tview.Frame, it *bigquery.RowIterator) {
	frame.Clear()
	frame.
		AddText(fmt.Sprintf("%v Query Results", it.TotalRows), true, tview.AlignCenter, tcell.ColorGreen).
		AddText(fmt.Sprintf("%v/%v page", currentPage+1, it.TotalRows/PAGE_SIZE+1), true, tview.AlignCenter, tcell.ColorOrange).
		AddText("Ctrl-C to Stop, j/k/l/h or Arrows to move selection, Enter to copy, Tab/S-Tab to move pages", false, tview.AlignCenter, tcell.ColorGreen)
}

func getNextPage(it *bigquery.RowIterator, page int) [][]string {
	var rows [][]string
	for i := 0; i < page; i++ {
		var values []bigquery.Value
		err := it.Next(&values)
		if err == iterator.Done {
			break
		}
		if err != nil {
			panic(err)
		}
		// it.Schema is only available after the first call to Next
		if i == 0 {
			header := make([]string, len(it.Schema))

			for i, field := range it.Schema {
				header[i] = field.Name + "[" + fmt.Sprintf("%v", field.Type) + "]"
			}
			rows = append(rows, header)
		}
		row := make([]string, len(values))
		for i, v := range values {
			row[i] = fmt.Sprintf("%v", v)
		}
		rows = append(rows, row)
	}
	return rows
}

func main() {
	query := ""
	if len(os.Args) > 1 {
		queryFile := os.Args[1]
		content, err := ioutil.ReadFile(queryFile)
		if err != nil {
			panic(err)
		}
		query = string(content)
	}
	if query == "" {
		content, err := ioutil.ReadAll(os.Stdin)
		if err != nil {
			panic(err)
		}
		query = string(content)
	}

	ctx := context.Background()
	creds, err := google.FindDefaultCredentials(ctx, bigquery.Scope)
	if err != nil {
		panic(err)
	}
	projectID := creds.ProjectID
	if projectID == "" {
		outJSON, err := exec.Command("gcloud", "-q", "config", "list", "core/project", "--format=json").Output()
		if err != nil {
			panic(err)
		}
		var m map[string]map[string]string
		json.Unmarshal(outJSON, &m)
		projectID = m["core"]["project"]
	}
	client, err := bigquery.NewClient(ctx, projectID)
	if err != nil {
		panic(err)
	}

	log.Println("project_id:", client.Project())
	log.Println("executing query:", query)
	q := client.Query(query)
	// job, err := q.Run(ctx)
	it, err := q.Read(ctx)
	if err != nil {
		panic(err)
	}

	app := tview.NewApplication()
	table := tview.NewTable().
		SetBorders(false).
		SetSelectable(true, true)

	page := getNextPage(it, PAGE_SIZE)
	pages = append(pages, page)
	for r, row := range page {
		color := tcell.ColorWhite
		if r == 0 {
			color = tcell.ColorTeal
		}
		for c, cell := range row {
			table.SetCell(r+1, c,
				tview.NewTableCell(cell).
					SetTextColor(color).
					SetAlign(tview.AlignLeft))
		}
	}

	frame := tview.NewFrame(table).
		SetBorders(2, 2, 2, 2, 4, 4)
	drawFrame(frame, it)
	table.Select(0, 0).SetFixed(1, 1).SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyCtrlC {
			app.Stop()
		}
		if key == tcell.KeyTAB {
			page := pages[0]
			if currentPage == len(pages)-1 {
				page = getNextPage(it, PAGE_SIZE)
				if len(page) == 0 {
					return
				}
				pages = append(pages, page)
			} else {
				page = pages[currentPage+1]
			}
			currentPage++
			drawPage(table, currentPage)
			drawFrame(frame, it)
		}
		if key == tcell.KeyBacktab {
			if currentPage == 0 {
				return
			}
			page = pages[currentPage-1]
			currentPage--
			drawPage(table, currentPage)
			drawFrame(frame, it)
		}
	}).SetSelectedFunc(func(row int, column int) {
		clipboard.WriteAll(table.GetCell(row, column).Text)
	})
	if err := app.SetRoot(frame, true).EnableMouse(true).Run(); err != nil {
		panic(err)
	}
}
