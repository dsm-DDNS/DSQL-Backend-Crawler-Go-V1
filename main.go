package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go/rand"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/chromedp"
	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"golang.org/x/net/html"
	"gopkg.in/yaml.v2"
)

var (
	url    string
	dbSets DbSettings
	db     *sqlx.DB
)

type S3Info struct {
	AwsS3Region  string `yaml:"aws_s3_region"`
	AwsAccessKey string `yaml:"aws_access_key"`
	AwsSecretKey string `yaml:"aws_secret_key"`
	BucketName   string `yaml:"bucket_name"`
	S3Client     *s3.Client
}

var s3Client S3Info
var uuid = rand.UUID{}

func parseS3FromConfig() S3Info {
	file, err := ioutil.ReadFile("./resources/s3.yaml")
	if err != nil {
		log.Fatalln(err.Error())
	}

	err = yaml.Unmarshal(file, &s3Client)
	if err != nil {
		log.Fatalln(err.Error())
	}
	return s3Client
}

type sel struct {
	selection *goquery.Selection
}

type DbSettings struct {
	Database struct {
		Username     string `yaml:"username"`
		Pw           string `yaml:"password"`
		DbUrl        string `yaml:"url"`
		DbName       string `yaml:"db_name"`
		DatabaseType string `yaml:"databaseType"`
	} `yaml:"database"`
}

func parseDbFromConfig() DbSettings {
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
	parseS3FromConfig()
	err := s3Client.SetS3ConfigByKey()

	dbSets = parseDbFromConfig()

	flag.StringVar(&url, "t", " ", "target to crawling")
	flag.Parse()

	dbSource := fmt.Sprintf(dbSets.Database.Username + ":" + dbSets.Database.Pw + "@(" + dbSets.Database.DbUrl + ")/" + dbSets.Database.DbName)

	open, err := sqlx.Open(dbSets.Database.DatabaseType, dbSource)
	err = open.Ping()
	if err != nil {
		log.Fatal(err.Error())
	}
	db = open

}

func main() {

	defer func(db *sqlx.DB) {
		err := db.Close()
		if err != nil {

		}
	}(db)

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
	if err != nil {
		println("ERROR:", err)
	}

	log.Println("PARSING DATA.....")
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(str))
	if err != nil {
		log.Fatalln(err.Error())
		return
	}

	s := sel{selection: doc.Selection}

	//content := strings.ReplaceAll(s.GetData(), "#", "")
	content := s.GetData()
	img := s.getImageSrc()

	InsertBlogContent(&url, &content, &img)

	if err != nil {
		panic(err)
	}

}

func (s *S3Info) SetS3ConfigByKey() error {
	creds := credentials.NewStaticCredentialsProvider(s.AwsAccessKey, s.AwsSecretKey, "")
	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithCredentialsProvider(creds),
		config.WithRegion(s.AwsS3Region),
	)
	if err != nil {
		log.Printf("error: %v", err)
		panic(err)
		return errors.New(err.Error())
	}
	s.S3Client = s3.NewFromConfig(cfg)
	return nil
}

func (s *S3Info) UploadFile(file io.Reader, filename, preFix string) *manager.UploadOutput {
	fileType := "image/" + findExtension(filename)
	uploader := manager.NewUploader(s.S3Client)
	result, err := uploader.Upload(context.TODO(), &s3.PutObjectInput{
		Bucket:      aws.String(s.BucketName),
		Key:         aws.String(preFix + "/" + filename),
		Body:        file,
		ContentType: &fileType,
	})
	if err != nil {
		log.Fatal(err)
	}
	return result
}

func findExtension(path string) string {
	ext := filepath.Ext(path)
	_, result, _ := strings.Cut(ext, ".")
	return result
}

//func ExistsBlogContentByUrl(url *string) bool {
//	var title string
//	sql := `SELECT title FROM POST WHERE url = ?`
//	err := db.QueryRow(sql, *url).Scan(&title)
//	if err != nil {
//		log.Fatal(err.Error())
//	}
//
//	println("[EXISTS]: ", title)
//
//	return true
//}

func InsertBlogContent(url, content, img *string) {
	var err error
	if len(*content+*img) == 0 {
		_, err = db.NamedExec(`UPDATE POST SET content=null, img=null WHERE url = :url`, map[string]interface{}{
			"url": url,
		})
	} else {
		_, err = db.NamedExec(`UPDATE POST SET content=:content, img=:img WHERE url = :url`,
			map[string]interface{}{
				"url":     url,
				"content": content,
				"img":     img,
			})
	}
	if err != nil {
		log.Fatal(err.Error())
	}

	log.Println("[INSERTED]: \n [contents]: ", *content, "\n [img]: ", *img+"\n")
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

func (s *sel) getImageSrc() string {
	var buf bytes.Buffer

	var f func(node *html.Node)

	f = func(node *html.Node) {
		if node.Data == "img" {
			img := node.Attr[0].Val
			if strings.Contains(img, "w80_blur") {
				img = strings.ReplaceAll(img, "w80_blur", "w773")
			}
			res, err := http.Get(img)
			if err != nil {
				panic(err)
			}

			defer func(Body io.ReadCloser) {
				err := Body.Close()
				if err != nil {

				}
			}(res.Body)

			reader, err := ioutil.ReadAll(res.Body)
			if err != nil {
				panic(err)
			}

			filename, _ := uuid.GetUUID()

			buf.WriteString(s3Client.UploadFile(bytes.NewReader(reader), filename, "post").Location + "\n")
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
