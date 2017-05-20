package main

import (
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
	mobiPromotionURL     = `http://mobifone.vn/wps/portal/public/khuyen-mai/tin-khuyen-mai`
	promoItemXPath       = `//div[@class="list-item-dem"]`
	promoItemHeaderXPath = `//div[@class="list-item-dem-header"]`
	promoItemBodyXPath   = `//div[@class="list-item-dem-description"]`

	mailFrom      = `ngdinhtoan@gmail.com`
	sentPromosKey = `sent_promos`
)

var (
	domain         = flag.String("mg-domain", os.Getenv("MG_DOMAIN"), "MailGun domain name")
	apiKey         = flag.String("mg-api-key", os.Getenv("MG_API_KEY"), "MailGun API key")
	publicAPIKey   = flag.String("mg-public-api-key", os.Getenv("MG_PUBLIC_API_KEY"), "MailGun public API key")
	hookPrivateKey = flag.String("hook-private-key", os.Getenv("HOOK_PRIVATE_KEY"), "Hook private key for DataStorage access")
	debug          = flag.Bool("debug", false, "Print some debug message")
)

var (
	mailTo = []string{
		"ngdinhtoan@gmail.com",
		// want to subscribe?
		// create pull request!
	}

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
	root, err := htmlquery.LoadURL(mobiPromotionURL)
	checkError(err)

	sentPromos, err := getSentPromos()
	checkError(err)

	if sentPromos == nil {
		sentPromos = make(map[string]int64)
	}

	defer func(sentPromos map[string]int64) {
		setSentPromos(sentPromos)
	}(sentPromos)

	v := &promoVisitor{
		sentPromos: sentPromos,
	}

	htmlquery.FindEach(root, promoItemXPath, v.checkPromoItem)
}

type promoVisitor struct {
	sentPromos map[string]int64
}

func (v *promoVisitor) checkPromoItem(i int, node *html.Node) {
	headerNode := htmlquery.FindOne(node, promoItemHeaderXPath)
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
	descNode := htmlquery.FindOne(node, promoItemBodyXPath)
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
	sendMessage(title, description)

	v.sentPromos[hashMsg] = time.Now().UTC().Unix()
}

func getSentPromos() (sentPromos map[string]int64, err error) {
	var result string
	if result, err = ds.Get(sentPromosKey); err != nil {
		return
	}

	sentPromos = make(map[string]int64)
	err = json.Unmarshal([]byte(result), &sentPromos)
	if err != nil && *debug {
		log.Println("--> Invalid JSON string:", result)
	}

	return
}

func setSentPromos(sentPromos map[string]int64) (err error) {
	var data []byte
	if data, err = json.Marshal(sentPromos); err != nil {
		return
	}

	err = ds.Set(sentPromosKey, string(data))
	return
}

func sendMessage(title, description string) {
	if description == "" {
		description = title
	}

	msg := mailgun.NewMessage(mailFrom, title, description, mailTo...)
	_, _, err := mg.Send(msg)
	checkError(err)
}

func hashMessage(msg string) (hash string, err error) {
	h := sha1.New()
	_, err = io.WriteString(h, msg)
	if err != nil {
		return
	}

	return fmt.Sprintf("%X", h.Sum(nil)), nil
}

func checkError(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
