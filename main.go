package main

import (
	"bufio"
	"crypto/sha1"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/antchfx/xquery/html"
	"github.com/kennygrant/sanitize"
	"golang.org/x/net/html"
	"gopkg.in/mailgun/mailgun-go.v1"
)

const (
	mailFrom            = `ngdinhtoan@gmail.com`
	sentPromosKeyPrefix = `sent_promos_`
)

var (
	debug          = flag.Bool("debug", false, "Print some debug message")
	domain         = flag.String("mg-domain", os.Getenv("MG_DOMAIN"), "MailGun domain name")
	apiKey         = flag.String("mg-api-key", os.Getenv("MG_API_KEY"), "MailGun API key")
	publicAPIKey   = flag.String("mg-public-api-key", os.Getenv("MG_PUBLIC_API_KEY"), "MailGun public API key")
	hookPrivateKey = flag.String("hook-private-key", os.Getenv("HOOK_PRIVATE_KEY"), "Hook private key for DataStorage access")
	mobiProvider   = flag.String("mobi-provider", "mobifone", "Mobi provider name: mobifone, viettel, vinaphone")
	recipientFile  = flag.String("recipient-file", "", "File to load recipients")
)

var (
	mg mailgun.Mailgun
	ds *hookDBClient
)

func init() {
	flag.Parse()

	mg = mailgun.NewMailgun(*domain, *apiKey, *publicAPIKey)
	ds = newStorage(*hookPrivateKey)

	log.SetPrefix("[mobi-promo-notify] ")
	if *debug {
		log.SetFlags(log.Lshortfile)
	} else {
		log.SetFlags(0)
	}
}

func main() {
	v := &promoVisitor{
		itemXPath:        `//div[contains(@class, "news_items")]`,
		titleXPath:       `//a[contains(@class, "entry-title")]`,
		descriptionXPath: `//div[contains(@class, "entry-summary")]`,
		mobiProvider:     *mobiProvider,
	}

	mailTo, err := readLines(*recipientFile)
	checkError(err)
	if len(mailTo) == 0 {
		log.Println("No subscriber")
		return
	}

	v.mailTo = mailTo

	switch *mobiProvider {
	case "mobifone":
		v.pageURL = `http://dichvudidong.vn/tin-khuyen-mai`
	case "viettel":
		v.pageURL = `http://dichvudidong.vn/tin-khuyen-mai-viettel`
	case "vinaphone":
		v.pageURL = `http://dichvudidong.vn/tin-khuyen-mai-vinaphone`
	default:
		log.Fatalln("Do not support mobile provider", *mobiProvider)
	}

	err = v.visit()
	checkError(err)
}

type promoVisitor struct {
	pageURL          string
	itemXPath        string
	titleXPath       string
	descriptionXPath string
	mobiProvider     string

	mailTo     []string
	sentPromos map[string]int64
}

func (v *promoVisitor) visit() (err error) {
	var root *html.Node
	root, err = htmlquery.LoadURL(v.pageURL)
	if err != nil {
		return
	}

	v.sentPromos, err = getSentPromos(sentPromosKeyPrefix + v.mobiProvider)
	if err != nil {
		return
	}

	if v.sentPromos == nil {
		v.sentPromos = make(map[string]int64)
	}

	defer func(key string, sentPromos map[string]int64) {
		setSentPromos(key, sentPromos)
	}(sentPromosKeyPrefix+v.mobiProvider, v.sentPromos)

	htmlquery.FindEach(root, v.itemXPath, v.checkPromoItem)
	return
}

func (v *promoVisitor) checkPromoItem(i int, node *html.Node) {
	headerNode := htmlquery.FindOne(node, v.titleXPath)
	if headerNode == nil {
		return
	}

	title := htmlquery.OutputHTML(headerNode, false)
	title = strings.TrimSpace(sanitize.HTML(title))
	if title == "" {
		if *debug {
			log.Println("[DEBUG] node content is empty", htmlquery.InnerText(node))
		}

		return
	}

	log.Println(title)

	if !strings.Contains(title, `50%`) {
		return
	}

	var description string
	descNode := htmlquery.FindOne(node, v.descriptionXPath)
	if descNode != nil {
		description = htmlquery.OutputHTML(descNode, false)
		description = strings.TrimSpace(sanitize.HTML(description))
	}

	hashMsg, err := hashMessage(title + description)
	checkError(err)

	if _, found := v.sentPromos[hashMsg]; found {
		log.Println("--> Promotion message has been notified")
		return
	}

	log.Println("--> Sending email:", title)
	err = sendMessage(title, description, v.mailTo...)
	checkError(err)
	v.sentPromos[hashMsg] = time.Now().UTC().Unix()
}

func getSentPromos(key string) (sentPromos map[string]int64, err error) {
	var result string
	if result, err = ds.Get(key); err != nil {
		return
	}

	sentPromos = make(map[string]int64)
	err = json.Unmarshal([]byte(result), &sentPromos)
	if err != nil && *debug {
		log.Println("--> Invalid JSON string:", result)
	}

	return
}

func setSentPromos(key string, sentPromos map[string]int64) (err error) {
	var data []byte
	if data, err = json.Marshal(sentPromos); err != nil {
		return
	}

	err = ds.Set(key, string(data))
	return
}

func sendMessage(title, description string, to ...string) (err error) {
	if len(to) == 0 {
		return
	}

	if description == "" {
		description = title
	}

	msg := mailgun.NewMessage(mailFrom, title, description, to...)
	_, _, err = mg.Send(msg)
	return
}

func hashMessage(msg string) (hash string, err error) {
	h := sha1.New()
	_, err = io.WriteString(h, msg)
	if err != nil {
		return
	}

	return fmt.Sprintf("%X", h.Sum(nil)), nil
}

// readLines reads a whole file into memory
// and returns a slice of its lines.
func readLines(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

func checkError(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
