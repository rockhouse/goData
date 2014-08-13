package goData

import (
	"errors"
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
		return
	}

	notationIDs, err := extractNotationIDs(txt)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		c.Errorf(err.Error())
		return
	}

	// Get azID
	txt, err = fetchContent(c, URLID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		c.Errorf("%v", err)
		return
	}

	azID := txt[strings.Index(txt, AuthID)+19 : strings.Index(txt, "==\";")+2]
	c.Debugf("API AZID: %v", azID)

	// Get user id
	//unixTime := (time.Now().UnixNano() / 1e6)
	unixTime := (time.Now().UnixNano() / 1e6)

	urlUserID, err := prepareURL(URLUserID, azID, strconv.FormatInt(unixTime, 10))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		c.Errorf("%v", err)
		return
	}

	txt, err = fetchContent(c, urlUserID)

	if len(txt) < 1 {
		c.Errorf("Got empty userID respones")
		http.Error(w, "Got empty userID respones", http.StatusInternalServerError)
		return
	}

	userID := strings.TrimSpace(strings.Split(txt, ";")[6])
	proxy := strings.TrimSpace(strings.Split(txt, ";")[9])
	if strings.Contains(userID, "-ZpUK.") {
		c.Errorf("Got wrong userID respones")
		http.Error(w, "Got wrong userID respones", http.StatusInternalServerError)
		return
	}
	c.Debugf("FOUND USERID: %v", userID)

	//Request Price
	urlReqPrice, err := prepareURL(URLReqPrice, azID, notationIDs[0], "Hr")
	postValue := urlReqPrice
	urlReqPrice, err = prepareURL(URLReqPrice, azID, notationIDs[1], "Ht")
	postValue += urlReqPrice
	urlReqPrice, err = prepareURL(URLReqPrice, azID, notationIDs[2], "Hv")
	postValue += urlReqPrice

	urlPrice, err := prepareURL(URLPrice, proxy, azID, userID)
	txt, err = postContent(c, urlPrice, postValue)

	fmt.Fprintf(w, "AZID: %v\nUSERID: %v\n", azID, userID)

	//Request Data Update
	urlUpdate, err := prepareURL(URLUpdate, proxy, azID, userID, strconv.FormatInt(time.Now().UnixNano()/1e6, 10))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		c.Errorf("%v", err)
		return
	}

	fmt.Fprintf(w, "Update URL: %v", urlUpdate)

	txt, err = fetchContent(c, urlUpdate)

	fmt.Fprintf(w, "CONTENT: %s", txt)

}

// Extracts the three NotationIDs from a given string. Returns an error
// it can not find three ID's
func extractNotationIDs(str string) ([]string, error) {
	startIndex := str[strings.Index(str, "@IdNotation(")+12:]
	isinAndNotation := startIndex[:strings.Index(startIndex, ")")]
	arr := strings.Split(isinAndNotation, "=")
	notationIDs := strings.Split(arr[2][1:len(arr[2])-1], ",")

	if len(notationIDs) < 3 {
		return nil, errors.New("Could not find Notational IDs")
	}
	return notationIDs, nil
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
	req.Header.Add("Referer", Referrer)
	//req.Header.Add("Host", HostHeader)
	client := urlfetch.Client(c)
	resp, err := client.Do(req)

	if err != nil {
		return "", err
	}

	defer resp.Body.Close()
	b, err := ioutil.ReadAll(resp.Body)
	// c.Debugf("URL Body: %s \nBODY END", b)
	return string(b), err
}

func prepareURL(tmpl string, values ...string) (string, error) {
	if tmpl == "" || values == nil {
		return "", errors.New("no arguments given")
	}
	count := strings.Count(tmpl, "[[?]]")

	if len(values) < count {
		return "", errors.New("too many parameters provided")
	} else if len(values) > count {
		return "", errors.New("too few parameters provided")
	}

	returnValue := tmpl
	for _, value := range values {
		returnValue = strings.Replace(returnValue, "[[?]]", value, 1)
	}

	return returnValue, nil
}
