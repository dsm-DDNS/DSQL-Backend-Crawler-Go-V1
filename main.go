package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/chromedp"
	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"golang.org/x/net/html"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
	"strings"
	"time"
)

var (
	url    string
	dbSets DbSettings
	db     *sqlx.DB
)

type sel struct {
	selection *goquery.Selection
}

type DbSettings struct {
	Database struct {
		Username     string `yaml:"username"`
		Pw           string `yaml:"password"`
		Addr         string `yaml:"address"`
		Port         string `yaml:"port"`
		DbName       string `yaml:"dbName"`
		DatabaseType string `yaml:"databaseType"`
	} `yaml:"database"`
}

func parseFromConfig() DbSettings {
	var set DbSettings
	file, err := ioutil.ReadFile("./resources/properties.yaml")
	if err != nil {
		panic("Failed to Read File")
	}

	err = yaml.Unmarshal(file, &set)
	if err != nil {
		panic("Failed to Marshal File")
	}
	return set
}

func init() {

	dbSets = parseFromConfig()
	flag.StringVar(&url, "t", " ", "target to crawling")
	flag.Parse()

	dbSource := fmt.Sprintf(dbSets.Database.Username + ":" + dbSets.Database.Pw + "@(" + dbSets.Database.Addr + ":" + dbSets.Database.Port + ")/" + dbSets.Database.DbName)

	open, err := sqlx.Open(dbSets.Database.DatabaseType, dbSource)
	err = open.Ping()
	if err != nil {
		log.Fatal(err.Error())
	}
	db = open
}

func main() {

	defer db.Close()

	log.Println("Target: {" + url + "}")

	if url == " " {
		return
	}
	//if ExistsBlogContentByUrl(&url) {
	//
	//}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.DisableGPU,
		chromedp.Flag("headless", true),
	)

	contextVar, cancelFunc := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelFunc()

	contextVar, cancelFunc = chromedp.NewContext(contextVar)
	defer cancelFunc()

	contextVar, cancelFunc = context.WithTimeout(contextVar, 8*time.Second)
	defer cancelFunc()

	var str string
	log.Println("SELECTING DATA.....")
	err := chromedp.Run(contextVar,
		chromedp.Navigate(url),
		chromedp.WaitVisible("se-main-container"),
		chromedp.OuterHTML("se-main-container", &str),
	)

	log.Println("PARSING DATA.....")
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(str))
	if err != nil {
		log.Fatalln(err.Error())
		return
	}

	s := sel{selection: doc.Selection}

	//content := strings.ReplaceAll(s.GetData(), "#", "")
	content := s.GetData()

	InsertBlogContent(&url, &content)

	if err != nil {
		panic(err)
	}

}

func ExistsBlogContentByUrl(url *string) bool {
	var title string
	sql := `SELECT title FROM post WHERE url = ?`
	err := db.QueryRow(sql, *url).Scan(&title)
	if err != nil {
		log.Fatal(err.Error())
	}

	println("[EXISTS]: ", title)

	return true
}

func InsertBlogContent(url, content *string) {
	var err error
	if len(*content) == 0 {
		_, err = db.NamedExec(`UPDATE post SET content=null WHERE url = :url`, map[string]interface{}{
			"url": url,
		})
	} else {
		_, err = db.NamedExec(`UPDATE post SET content=:content WHERE url = :url`,
			map[string]interface{}{
				"url":     url,
				"content": content,
			})
	}
	if err != nil {
		log.Fatal(err.Error())
	}

	log.Println("[INSERTED]: \n", *content)
}

func (s *sel) GetData() string {
	var buf bytes.Buffer

	var f func(*html.Node)

	f = func(node *html.Node) {
		//for _, i := range node.Attr {
		//	//if i.Key == "href" {
		//	//	link := strings.ReplaceAll(strings.ReplaceAll(i.Val, "\n", ""), " ", "")
		//	//	if strings.Contains(buf.String(), link) {
		//	//		continue
		//	//	} else {
		//	//		println("Href Add: {", link, "}")
		//	//		buf.WriteString(link + "\n")
		//	//	}
		//	//}
		//	//
		//	if i.Key == "href" {
		//		return
		//	}
		//	//if i.Val == "se-section se-section-oglink se-l-image se-section-align-" {
		//	//	return
		//	//}
		//}
		if node.Data == "img" {
			img := node.Attr[0].Val
			if strings.Contains(img, "w80_blur") {
				img = strings.ReplaceAll(img, "w80_blur", "w773")
			}
			buf.WriteString(img + "\n")
		}

		if node.Type == html.TextNode {
			if !strings.Contains(node.Data, "\n") {
				data := node.Data
				//if strings.Contains(data, "https://") {
				//	return
				//}
				buf.WriteString(data + "\n")
			}
		}

		if node.FirstChild != nil {
			for c := node.FirstChild; c != nil; c = c.NextSibling {
				f(c)
			}
		}
	}

	for _, n := range s.selection.Nodes {
		f(n)
	}

	return buf.String()
}
