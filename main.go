package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/savioxavier/termlink"
	_ "modernc.org/sqlite"
)

var markdown_dir_path string
var mdPrefix, mdSuffix string
var terminal_mode bool = false
var currentDate = time.Now().Format("2006-01-02")
var lat, lon float64
var instapaper bool
var openaiApiKey string
var myMap []RSS
var db *sql.DB

type RSS struct {
	url       string
	limit     int
	summarize bool
}

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func writeLink(title string, url string, newline bool) string {
	var content string
	if terminal_mode {
		content = termlink.Link(title, url)
		if newline {
			content += "\n"
		}
	} else {
		content = "[" + title + "](" + url + ")"
		if newline {
			content += "\n"
		}
	}
	return content
}

func writeSummary(content string, newline bool) string {
	if content == "" {
		return content
	}
	if terminal_mode {
		if newline {
			content += "\n"
		}
	} else {
		if newline {
			content += "  \n\n"
		}
	}
	return content
}

func favicon(s *gofeed.Feed) string {
	if terminal_mode {
		return ""
	}
	var src string
	if s.FeedLink == "" {
		// default feed favicon
		return "🍵"

	} else {
		u, err := url.Parse(s.FeedLink)
		if err != nil {
			fmt.Println(err)
		}
		src = "https://www.google.com/s2/favicons?sz=32&domain=" + u.Hostname()
	}
	// if s.Title contains "hacker news"
	if strings.Contains(s.Title, "Hacker News") {
		src = "https://news.ycombinator.com/favicon.ico"
	}

	//return html image tag of favicon
	return fmt.Sprintf("<img src=\"%s\" width=\"32\" height=\"32\" />", src)
}

func writeToMarkdown(body string) {
	if terminal_mode {
		fmt.Println(body)
	} else {
		markdown_file_name := mdPrefix + currentDate + mdSuffix + ".md"
		f, err := os.OpenFile(filepath.Join(markdown_dir_path, markdown_file_name), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Fatal(err)
		}
		if _, err := f.Write([]byte(body)); err != nil {
			log.Fatal(err)
		}
		if err := f.Close(); err != nil {
			log.Fatal(err)
		}
	}
}

func parseFeedURL(fp *gofeed.Parser, url string) *gofeed.Feed {
	feed, err := fp.ParseURL(url)
	if err != nil {
		return nil
	}
	return feed
}

func main() {
	bootstrapConfig()

	// Start writing to markdown
	// Display weather if lat and lon are set
	if lat != 0 && lon != 0 {
		writeToMarkdown(getWeather(lat, lon))
	}

	fp := gofeed.NewParser()
	for _, rss := range myMap {
		feed := parseFeedURL(fp, rss.url)

		if feed == nil {
			continue
		}

		items := ""
		for index, item := range feed.Items {
			if index == rss.limit {
				break
			}
			var url string
			var date string
			var summary sql.NullString
			var summaryValue string

			title := item.Title
			link := item.Link

			err := db.QueryRow("SELECT url, date, summary FROM seen where url=?", item.Link).Scan(&url, &date, &summary)
			if err != nil && err != sql.ErrNoRows {
				fmt.Println(err)
			}
			if summary.Valid {
				summaryValue = summary.String
			} else {
				summaryValue = ""
			}

			if url != "" && date == currentDate {
				// fmt.Println("Already seen: " + item.Title)
				// Article is already in the database and it is for today's date so skip inserting it
			} else if url != "" && date != currentDate {
				// fmt.Println("Skipping: " + item.Link)
				continue
			} else {
				if rss.summarize {
					summaryValue = getSummaryFromLink(link)
				} else {
					summaryValue = ""
				}
				stmt, err := db.Prepare("INSERT INTO seen(url, date, summary) values(?,?,?)")
				check(err)
				res, err := stmt.Exec(item.Link, currentDate, summaryValue)
				check(err)
				_ = res
				stmt.Close()
			}

			if strings.Contains(feed.Title, "Hacker News") {
				// Find Comments URL
				first_index := strings.Index(item.Description, "Comments URL") + 23
				comments_url := item.Description[first_index : first_index+45]
				// Find Comments number
				first_comments_index := strings.Index(item.Description, "Comments:") + 10
				// replace </p> with empty string
				comments_number := strings.Replace(item.Description[first_comments_index:], "</p>\n", "", -1)
				comments_number_int, _ := strconv.Atoi(comments_number)
				if comments_number_int < 100 {
					items += writeLink("💬 ", comments_url, false)
				} else {
					items += writeLink("🔥 ", comments_url, false)
				}
			}
			if instapaper && !terminal_mode {
				items += "[<img height=\"16\" src=\"https://staticinstapaper.s3.dualstack.us-west-2.amazonaws.com/img/favicon.png\">](https://www.instapaper.com/hello2?url=" + item.Link + ")"
			}

			// Support RSS with no Title (such as Mastodon), use Description instead
			if title == "" {
				title = stripHtmlRegex(item.Description)
			}
			items += writeLink(title, link, true)
			if rss.summarize {
				items += writeSummary(summaryValue, true)
			}
		}

		if items != "" {
			writeToMarkdown("\n### " + favicon(feed) + "  " + feed.Title + "\n")
			writeToMarkdown(items)
		}
		defer db.Close()

	}
}
