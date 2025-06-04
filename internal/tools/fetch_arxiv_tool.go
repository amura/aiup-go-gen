package tools

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"aiupstart.com/go-gen/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type FetchArxivTool struct{}

type arxivFeed struct {
	Entries []struct {
		Title   string `xml:"title"`
		Authors []struct {
			Name string `xml:"name"`
		} `xml:"author"`
		Summary string `xml:"summary"`
		ID      string `xml:"id"`
	} `xml:"entry"`
}

func (t *FetchArxivTool) Name() string { return "fetch_arxiv" }
func (t *FetchArxivTool) Description() string {
    return "Fetch recent arXiv papers on a given topic."
}
func (t *FetchArxivTool) Parameters() map[string]string {
    return map[string]string{"query": "string"}
}
func (t *FetchArxivTool) Call(ctx context.Context, call ToolCall) ToolResult {
	metrics.ToolCallsTotal.WithLabelValues(t.Name(), call.Caller).Inc()
	timer := prometheus.NewTimer(metrics.ToolLatencySeconds.WithLabelValues(t.Name(), call.Caller))
	defer timer.ObserveDuration()
	
    query, ok := call.Args["query"].(string)
    if !ok {
        return ToolResult{Error: fmt.Errorf("missing argument: query")}
    }
	

	apiURL := "http://export.arxiv.org/api/query?search_query=" + url.QueryEscape(query) + "&start=0&max_results=5&sortBy=submittedDate&sortOrder=descending"
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return ToolResult{Error: err}
	}
	req.Header.Set("User-Agent", "aiup-go-gen/1.0")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ToolResult{Error: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ToolResult{Error: err}
	}
	var feed arxivFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return ToolResult{Error: err}
	}
	papers := []map[string]string{}
	for _, entry := range feed.Entries {
		authors := []string{}
		for _, a := range entry.Authors {
			authors = append(authors, a.Name)
		}
		papers = append(papers, map[string]string{
			"Title":   strings.TrimSpace(entry.Title),
			"Authors": strings.Join(authors, ", "),
			"Summary": strings.TrimSpace(entry.Summary),
			"URL":     entry.ID,
		})
	}
    // papers := []map[string]string{
    //     {"Title": fmt.Sprintf("LLM Research about %s", query), "Authors": "Jane Doe", "Summary": "Summary...", "URL": "http://arxiv.org/abs/1234"},
    // }
    return ToolResult{Output: papers}
}