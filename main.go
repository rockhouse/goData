package goData

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"appengine"
	"appengine/datastore"
	"appengine/delay"
	"appengine/urlfetch"
)

var assets = []string{"4744-18954/18956", //one Asset
	"4744-18954/18956", // another asset
	"4744-18954/18956"}

type DatastoreEntry struct {
	Timestamp  int64
	BlobValues string
	Update     bool
}

var scheduleNextUpdate *delay.Function

func init() {
	scheduleNextUpdate = delay.Func("key", update)
	http.HandleFunc("/", help)
	http.HandleFunc("/startUpdates", startUpdates)
}

func getUserID(c appengine.Context, azID string) (userID string,
	proxy string, err error) {
	unixTime := (time.Now().UnixNano() / 1e6)

	urlUserID, err := prepareURL(URLUserID, azID, strconv.FormatInt(unixTime, 10))
	if err != nil {
		return "", "", err
	}

	txt, err := fetchContent(c, urlUserID)
	if len(txt) < 1 {
		return "", "", errors.New("Got empty userID respones")
	}

	userID = strings.TrimSpace(strings.Split(txt, ";")[6])
	proxy = strings.TrimSpace(strings.Split(txt, ";")[9])
	if strings.Contains(userID, "-ZpUK.") {
		c.Errorf("Got problematic userID respones %s", userID)
	}
	c.Debugf("FOUND USERID: %v", userID)

	return userID, proxy, nil
}

func getPrice(c appengine.Context, azID string, notationIDs []string,
	proxy string, userID string) (string, error) {
	//Request Price
	urlReqPrice, err := prepareURL(URLReqPrice, azID, notationIDs[0], "Hr")
	postValue := urlReqPrice
	urlReqPrice, err = prepareURL(URLReqPrice, azID, notationIDs[1], "Ht")
	postValue += urlReqPrice
	urlReqPrice, err = prepareURL(URLReqPrice, azID, notationIDs[2], "Hv")
	postValue += urlReqPrice

	urlPrice, err := prepareURL(URLPrice, proxy, azID, userID)
	if err != nil {
		return "", err
	}
	return postContent(c, urlPrice, postValue)
}

func getAzID(c appengine.Context, asset string) (azID string, notationIDs []string, err error) {
	// Get ID Notation (IDs for underlying by expiry)
	urlIDNotation, err := prepareURL(URLIDNOTATION, asset)
	if err != nil {
		return "", nil, err
	}
	txt, err := fetchContent(c, urlIDNotation)
	if err != nil {
		return "", nil, err
	}

	notationIDs, err = extractNotationIDs(txt)
	if err != nil {
		return "", nil, err
	}

	// Get the final azID
	txt, err = fetchContent(c, URLID)
	if err != nil {
		return "", nil, err
	}

	azID = txt[strings.Index(txt, AuthID)+19 : strings.Index(txt, "==\";")+2]

	c.Debugf("API AZID: %v", azID)
	return azID, notationIDs, nil
}

func initiate(c appengine.Context) (azIDs, proxies, userIDs []string, err error) {
	var (
		// azIDs          []string   = make([]string, len(assets))
		azsNotationIDs [][]string = make([][]string, len(assets))
		// proxy          string     = ""
		// userIDs        []string   = make([]string, len(assets))
	)

	for _, asset := range assets {
		azID, notationIDs, err := getAzID(c, asset)
		if err != nil {
			c.Errorf("%v", err)
		}
		azIDs = append(azIDs, azID)
		azsNotationIDs = append(azsNotationIDs, notationIDs)
	}

	for _, azID := range azIDs {
		userID, proxy, err := getUserID(c, azID)
		if err != nil {
			c.Errorf("%v", err)
		}
		userIDs = append(userIDs, userID)
		proxies = append(proxies, proxy)
	}

	for i := range assets {
		price, err := getPrice(c, azIDs[i], azsNotationIDs[i],
			proxies[i], userIDs[i])
		if err != nil {
			return nil, nil, nil, err
		}
		unixTime := (time.Now().UnixNano() / 1e6)
		err = storeData(unixTime, price, false, c)
		if err != nil {
			return nil, nil, nil, err
		}
	}

	//Request Data Update
	// TODO: Needless? The same is done in Update or?
	// urlUpdate, err := prepareURL(URLUpdate, proxy, azID, userID,
	// 	strconv.FormatInt(time.Now().UnixNano()/1e6, 10))
	// if err != nil {
	// 	c.Errorf("%v", err)
	// 	return
	// }
	//
	// txt, err = fetchContent(c, urlUpdate)
	// err = storeData((time.Now().UnixNano() / 1e6), txt, true, c)
	// if err != nil {
	// 	c.Errorf("Storing error: %v", err)
	// 	return
	// }

	return azIDs, proxies, userIDs, nil
}

func help(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Available Commands:\n /init \n /update \n")
	fmt.Fprint(w, "Available Commands:\n /init \n /update \n")
	log.Print("TEST ERROR MESSAGE WITHOUT REQUEST")
}

func update(c appengine.Context, azID string, proxy string, userID string) {
	if proxy == "" || azID == "" || userID == "" {
		c.Debugf("ERROR: proxy, azID or userID unknown. Did you run init first?")
		return
	}

	pushUpdate, err := prepareURL(PushUpdate, proxy, azID, userID,
		strconv.FormatInt(time.Now().UnixNano()/1e6, 10))
	if err != nil {
		c.Errorf("%v", err)
		return
	}

	txt, err := fetchContent(c, pushUpdate)
	err = storeData((time.Now().UnixNano() / 1e6), txt, true, c)
	if err != nil {
		c.Errorf("Storing error: %v", err)
		return
	}

	c.Debugf("Got Values: %s", txt)

	if strings.Contains(txt, "421 InvalidPushClientId") {
		c.Errorf("Invalid Push Client ID - Start init again")
		time.Sleep(1000 * time.Millisecond)
		//Let's initialize again
		azID, proxy, userID, err := initiate(c)
		if err != nil {
			c.Errorf("%v", err)
			return
		}
		scheduleNextUpdate.Call(c, azID, proxy, userID)
		return
	}
	//Is it time to stop working?
	t, err := timeToDie(time.Now())
	if t {
		return
	}
	if err != nil {
		c.Errorf("Error with TimeZone: %v", err)
		return
	}

	time.Sleep(1000 * time.Millisecond)
	//Is it time to stop and go home?

	scheduleNextUpdate.Call(c, azID, proxy, userID)
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

func startUpdates(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	azIDs, proxy, userID, err := initiate(c)
	if err != nil {
		c.Errorf("%v", err)
		return
	}
	scheduleNextUpdate.Call(c, azIDs, proxy, userID)
}

func storeData(unixTime int64, txt string, update bool,
	c appengine.Context) error {

	if unixTime == 0 || txt == "" || c == nil {
		return errors.New("no arguments given")
	}

	txt = fmt.Sprintf("%0.400s", txt)
	blob := DatastoreEntry{
		Timestamp:  unixTime,
		BlobValues: txt,
		Update:     false,
	}

	incompleteKey := datastore.NewIncompleteKey(c, "datastoreEntry", nil)
	key, err := datastore.Put(c, incompleteKey, &blob)

	c.Debugf("Datastore key: %v", key)

	if err != nil {
		return err
	}
	return nil
}

func timeToDie(t time.Time) (bool, error) {
	//Time should be UTC! So CET should be 22:00
	if t.Location() != time.UTC {
		return false, errors.New("wrong Timezone!")
	}
	if t.Hour() >= 20 && t.Minute() >= 01 {
		return true, nil
	}
	return false, nil
}
