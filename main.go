package goData

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"

	"appengine"
	"appengine/urlfetch"
)

func init() {
	http.HandleFunc("/", handler)

}

//check push
func handler(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)

	// Get ID Notation (IDs for underlying by expiry)
	txt, err := fetchContent(c, URLIDNOTATION)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		c.Errorf("%v", err)
	}
	startIndex := txt[strings.Index(txt, "@IdNotation(")+12:]
	IsinAndNotation := startIndex[:strings.Index(startIndex, ")")]
	IDArray := strings.Split(IsinAndNotation, "=")
	NotationIDs := strings.Split(IDArray[2][1:len(IDArray[2])-1], ",")

	if len(NotationIDs) < 3 {
		http.Error(w, "Could not find Notational IDs", http.StatusInternalServerError)
		c.Errorf("Could not find Notational IDs")
	}

	// Get azID
	txt, err = fetchContent(c, URLID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		c.Errorf("%v", err)
	}

	azID := txt[strings.Index(txt, AuthID)+19 : strings.Index(txt, "==\";")+2]
	c.Debugf("API AZ ID: %v", azID)

	// Get user id
	unixTime := time.Now().Unix()
	urlUserID := URLUserID
	urlUserID = strings.Replace(urlUserID, "[[AZID]]", azID, 1)
	urlUserID = strings.Replace(urlUserID, "[[UNIXTIME]]", strconv.FormatInt(unixTime, 10), 1)
	txt, err = fetchContent(c, urlUserID)

	if len(txt) < 1 {
		c.Errorf("Got empty userID respone")
		http.Error(w, "Got empty userID respone", http.StatusInternalServerError)
		return
	}

	userID := strings.TrimSpace(strings.Split(txt, ";")[6])
	proxy := strings.TrimSpace(strings.Split(txt, ";")[9])
	if strings.Contains(userID, "-ZpUK.") {
		c.Errorf("Got wrong userID respone")
		http.Error(w, "Got wrong userID respone", http.StatusInternalServerError)
		return
	}
	c.Debugf("FOUND USERID: %v", userID)

	//Request Price
	urlReqPrice := URLReqPrice
	urlReqPrice = strings.Replace(urlReqPrice, "[[AZID]]", azID, 1)
	urlReqPrice = strings.Replace(urlReqPrice, "[[UNDERLYING]]", "Hr", 1)
	urlReqPrice = strings.Replace(urlReqPrice, "[[IDNOTATION]]", NotationIDs[0], 1)
	postValue := urlReqPrice
	urlReqPrice = URLReqPrice
	urlReqPrice = strings.Replace(urlReqPrice, "[[AZID]]", azID, 1)
	urlReqPrice = strings.Replace(urlReqPrice, "[[UNDERLYING]]", "Ht", 1)
	urlReqPrice = strings.Replace(urlReqPrice, "[[IDNOTATION]]", NotationIDs[1], 1)
	postValue += urlReqPrice
	urlReqPrice = URLReqPrice
	urlReqPrice = strings.Replace(urlReqPrice, "[[AZID]]", azID, 1)
	urlReqPrice = strings.Replace(urlReqPrice, "[[UNDERLYING]]", "Hv", 1)
	urlReqPrice = strings.Replace(urlReqPrice, "[[IDNOTATION]]", NotationIDs[2], 1)
	postValue += urlReqPrice

	urlPrice := URLPrice
	urlPrice = strings.Replace(urlPrice, "[[AZID]]", azID, 1)
	urlPrice = strings.Replace(urlPrice, "[[USERID]]", userID, 1)
	urlPrice = strings.Replace(urlPrice, "[[PROXY]]", proxy, 1)
	txt, err = postContent(c, urlPrice, postValue)

	fmt.Fprintf(w, "AZID: %v\nUSERID: %v\n", azID, userID)

	//Request Data Update

	urlUpdate := URLUpdate
	urlUpdate = strings.Replace(urlUpdate, "[[AZID]]", azID, 1)
	urlUpdate = strings.Replace(urlUpdate, "[[USERID]]", userID, 1)
	urlUpdate = strings.Replace(urlUpdate, "[[PROXY]]", proxy, 1)
	urlUpdate = strings.Replace(urlUpdate, "[[TIME]]", strconv.FormatInt(time.Now().UnixNano()/1e6, 10), 1)

	fmt.Fprintf(w, "URL: %v", urlUpdate)

	txt, err = fetchContent(c, urlUserID)

	fmt.Fprintf(w, "CONTENT: %s", txt)

}

func postContent(c appengine.Context, url string, content string) (string, error) {
	c.Debugf("Post URL: %v with Content: %v", url, content)
	req, err := http.NewRequest("POST", url, strings.NewReader(content))
	req.Header.Add("Content-Type", "text/plain; charset=UTF-8")
	if err != nil {
		return "", err
	}
	return sendReq(c, req)
}

func fetchContent(c appengine.Context, url string) (string, error) {
	c.Debugf("Fetching URL: %v", url)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	return sendReq(c, req)
}

func sendReq(c appengine.Context, req *http.Request) (string, error) {
	req.Header.Add("User-Agent", UserAgent)
	client := urlfetch.Client(c)
	resp, err := client.Do(req)

	if err != nil {
		return "", err
	}

	defer resp.Body.Close()
	b, err := ioutil.ReadAll(resp.Body)
	c.Debugf("URL Body: %s \nBODY END", b)
	return string(b), err
}
