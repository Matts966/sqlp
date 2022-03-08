package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"time"

	"cloud.google.com/go/bigquery"
	"github.com/alecthomas/chroma/v2/quick"
	"github.com/atotto/clipboard"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/tj/go-spin"
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
var cacheHit = false
var isFetching = false

const PAGE_SIZE = 30

func drawPage(table *tview.Table, page int) {
	table.Clear()
	for r, row := range pages[page] {
		color := tcell.ColorWhite
		if r == 0 {
			color = tcell.ColorTeal
		}
		if r == 1 || r == 2 {
			color = tcell.ColorGreen
		}
		for c, cell := range row {
			table.SetCell(r+1, c,
				tview.NewTableCell(cell).
					SetTextColor(color))
		}
	}
}

func drawFrame(frame *tview.Frame, it *bigquery.RowIterator) {
	totalPages := it.TotalRows / PAGE_SIZE
	if it.TotalRows%PAGE_SIZE > 0 {
		totalPages++
	}
	frame.Clear()
	frame.
		AddText(fmt.Sprintf("%v Query Results", it.TotalRows), true, tview.AlignCenter, tcell.ColorGreen).
		AddText(fmt.Sprintf("%v/%v page [cacheHit=%v]", currentPage+1, totalPages, cacheHit), true, tview.AlignCenter, tcell.ColorOrange).
		AddText("Ctrl-C to Stop, j/k/l/h or Arrows to move selection, Enter to copy, Tab/S-Tab to move pages", false, tview.AlignCenter, tcell.ColorGreen)
}

type loadingFrameWriter struct {
	frame *tview.Frame
	it    *bigquery.RowIterator
	app   *tview.Application
}

func (l *loadingFrameWriter) Write(p []byte) (n int, err error) {
	l.app.QueueUpdateDraw(func() {
		drawFrame(l.frame, l.it)
		l.frame.AddText(string(p), true, tview.AlignLeft, tcell.ColorTeal)
	})
	return len(p), nil
}

func NewLoadingFrameWriter(app *tview.Application, frame *tview.Frame, it *bigquery.RowIterator) *loadingFrameWriter {
	return &loadingFrameWriter{
		frame: frame,
		it:    it,
		app:   app,
	}
}

type spinner struct {
	w io.Writer
	c chan struct{}
}

func NewSpinner(w io.Writer, message string, withColor bool) *spinner {
	done := make(chan struct{})
	s := spin.New()
	go func() {
		for {
			select {
			case <-done:
				return
			case <-time.After(time.Millisecond * 100):
				if withColor {
					fmt.Fprintf(w, "\r\033[36m%s\033[m %s ", message, s.Next())
				} else {
					fmt.Fprintf(w, "%s %s ", message, s.Next())
				}
			}
		}
	}()
	return &spinner{
		c: done,
		w: w,
	}
}

func (s *spinner) Done() {
	close(s.c)
	fmt.Fprintf(s.w, "\r")
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
			columns := make([]string, len(it.Schema))
			types := make([]string, len(it.Schema))
			modes := make([]string, len(it.Schema))
			for i, field := range it.Schema {
				columns[i] = field.Name
				types[i] = fmt.Sprintf("%s", field.Type)
				modes[i] = fmt.Sprintf("REQUIRED: %v", field.Required)
			}
			rows = append(rows, columns)
			rows = append(rows, types)
			rows = append(rows, modes)
		}
		row := make([]string, len(values))
		for i, v := range values {
			if v == nil {
				row[i] = "NULL"
				continue
			}
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
	log.Println("finished google.FindDefaultCredentials")
	projectID := creds.ProjectID
	if projectID == "" {
		outJSON, err := exec.Command("gcloud", "-q", "config", "list", "core/project", "--format=json").Output()
		if err != nil {
			panic(err)
		}
		var m map[string]map[string]string
		json.Unmarshal(outJSON, &m)
		projectID = m["core"]["project"]
		log.Println("finished retrieving projectID from gcloud command")
	}
	client, err := bigquery.NewClient(ctx, projectID)
	if err != nil {
		panic(err)
	}

	app := tview.NewApplication()
	table := tview.NewTable().
		SetBorders(false).
		SetSelectable(true, true)
	log.Println("finished tview.NewApplication")

	log.Println("project_id:", client.Project())
	log.Println("executing the query below")
	err = quick.Highlight(os.Stdout, query, "sql", "terminal", "monokai")
	fmt.Println()
	if err != nil {
		log.Println(err)
	}

	q := client.Query(query)
	job, err := q.Run(ctx)
	if err != nil {
		panic(err)
	}
	log.Println("finishd query.Run")
	spinner := NewSpinner(os.Stdout, "computing...", true)
	it, err := job.Read(ctx)
	spinner.Done()
	log.Println("finishd job.Read")
	if err != nil {
		panic(err)
	}

	spinner = NewSpinner(os.Stdout, "rendering...", true)
	page := getNextPage(it, PAGE_SIZE)
	pages = append(pages, page)
	drawPage(table, 0)

	frame := tview.NewFrame(table).
		SetBorders(2, 2, 2, 2, 4, 4)
	status, err := job.Status(ctx)
	if err != nil {
		panic(err)
	}
	cacheHit = status.Statistics.Details.(*bigquery.QueryStatistics).CacheHit
	drawFrame(frame, it)
	lfw := NewLoadingFrameWriter(app, frame, it)
	table.Select(0, 0).SetFixed(4, 0).SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyCtrlC {
			app.Stop()
		}
		if isFetching {
			return
		}
		if key == tcell.KeyTAB {
			page := pages[0]
			if currentPage == len(pages)-1 {
				spinner := NewSpinner(lfw, "fetching...", false)
				isFetching = true
				go func() {
					page = getNextPage(it, PAGE_SIZE)
					if len(page) == 0 {
						return
					}
					pages = append(pages, page)
					isFetching = false
					currentPage++
					drawPage(table, currentPage)
					drawFrame(frame, it)
					spinner.Done()
				}()
				return
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
		text := table.GetCell(row, column).Text
		clipboard.WriteAll(text)
		drawFrame(frame, it)
		frame.AddText(fmt.Sprintf("Copied to clipboard!: %v", text), false, tview.AlignCenter, tcell.ColorOrange)
	})
	spinner.Done()
	if err := app.SetRoot(frame, true).EnableMouse(true).Run(); err != nil {
		panic(err)
	}
}
