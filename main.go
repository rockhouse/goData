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
	"appengine/delay"
	"appengine/memcache"
	"appengine/urlfetch"
)

var assets = []string{"4744-18954/18956", //one Asset
	"4744-18954/18956", // another asset
}

type AssetParams struct {
	AssetID string
	AzID    string
	Proxy   string
	UserID  string
}

func saveDataEntry(c appengine.Context, assetID string, payload string) {

	// get a better log-format
	payload = strings.Replace(payload, "\n", "", -1) // Remove all newlines to

	//Write to log
	//Get logfile with: appcfg.py request_logs --severity=0 -n 1 . mylogs.txt
	c.Infof("ASSETDATA %s: %s", assetID, payload)

	// write meta to memcache
	a := NewAssetMeta(assetID)
	a.Cache(c)
}

var scheduleNextUpdate *delay.Function

func init() {
	scheduleNextUpdate = delay.Func("updateQueue", updateTask)
	http.HandleFunc("/", help)
	http.HandleFunc("/startUpdates", startUpdateQueues)
	http.HandleFunc("/age", timeSiceLastCache)
}

func initiateAsset(c appengine.Context, assetID string) (AssetParams, error) {
	azID, notationIDs, err := getAzID(c, assetID)
	if err != nil {
		c.Errorf("%v", err)
	}

	userID, proxy, err := getUserID(c, azID)
	if err != nil {
		c.Errorf("%v", err)
	}

	price, err := getPrice(c, azID, notationIDs,
		proxy, userID)
	if err != nil {
		return AssetParams{}, err
	}
	c.Debugf("PRICE RESPONSE: %v", price)

	return AssetParams{assetID, azID, proxy, userID}, nil
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
	c.Debugf("getPrice with\nazID %v\nnotationIDs %v\nproxy %v\n userid %v\n", azID, notationIDs, proxy, userID)
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
	txt, err := postContent(c, urlPrice, postValue)
	c.Debugf("Got %v", txt, err)
	return txt, err
}

func getAzID(c appengine.Context, asset string) (azID string, notationIDs []string, err error) {
	// Get ID Notation (IDs for underlying by expiry)
	urlIDNotation, err := prepareURL(URLIDNOTATION, asset)

	c.Debugf("urlIDNotation %s", URLIDNOTATION)
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

func help(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Available Commands:\n /startUpdates \n")
}

func updateTask(c appengine.Context, params AssetParams) {
	update(c, params)

	//Is it time to stop and go home?
	t, err := timeToDie(time.Now())
	if t || err != nil {

	}
	//Guarantees that a new update call is send even if urlfetch returns
	//an error or throws a timeout
	defer func() {
		time.Sleep(1000 * time.Millisecond)
		scheduleNextUpdate.Call(c, params)
	}()
}

func update(c appengine.Context, asset AssetParams) {
	if asset.Proxy == "" || asset.AzID == "" || asset.UserID == "" {
		c.Debugf("ERROR: proxy, azID or userID unknown. Did you run init first?")
		return
	}

	pushUpdate, err := prepareURL(PushUpdate, asset.Proxy, asset.AzID, asset.UserID,
		strconv.FormatInt(time.Now().UnixNano()/1e6, 10))
	if err != nil {
		c.Errorf("%v", err)
		return
	}

	txt, err := fetchContent(c, pushUpdate)
	saveDataEntry(c, asset.AssetID, txt)
	//TODO
	// if strings.Contains(txt, "421 InvalidPushClientId") {
	// 	c.Errorf("Invalid Push Client ID - Start init again")
	// 	time.Sleep(1000 * time.Millisecond)
	// 	//Let's initialize again
	// 	assetParams, err := initiateAsset(c, assets[0])
	// 	if err != nil {
	// 		c.Errorf("%v", err)
	// 		return
	// 	}
	// 	scheduleNextUpdate.Call(c, azID, proxy, userID)
	// 	return
	// }
	//Is it time to stop working?
	// if err != nil {
	// 	c.Errorf("Error with TimeZone: %v", err)
	// 	return
	// }

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
	txt, err := sendReq(c, req)
	return txt, err
}

func fetchContent(c appengine.Context, url string) (string, error) {
	// c.Debugf("Fetching URL: %v", url)
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
		return "", fmt.Errorf("no arguments given for %0.20s", tmpl)
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

func startUpdateQueues(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	for i := range assets {
		params, err := initiateAsset(c, assets[i])
		if err != nil {
			c.Errorf("Errors during inital of asset %s Error: %s", assets[i], err)
		}
		scheduleNextUpdate.Call(c, params)
	}
}

// func storeData(unixTime int64, txt string, update bool,
// 	c appengine.Context) error {
//
// 	if unixTime == 0 || txt == "" || c == nil {
// 		return fmt.Errorf("Unable to store. One of the folling values was empty."+
// 			"unixTime=%u, txt=%s, c=%v", unixTime, txt, c)
// 	}
//
// 	txt = fmt.Sprintf("%0.400s", txt)
// 	blob := DatastoreEntry{
// 		Timestamp:  unixTime,
// 		BlobValues: txt,
// 		Update:     false,
// 	}
//
// 	incompleteKey := datastore.NewIncompleteKey(c, "datastoreEntry", nil)
// 	_, err := datastore.Put(c, incompleteKey, &blob)
//
// 	if err != nil {
// 		return err
// 	}
// 	return nil
// }

//timeToDie checks if the current time is after CET 22:00 and stops the task
func timeToDie(t time.Time) (bool, error) {
	//Time should be UTC! So CET should be 22:00
	if t.Location() != time.UTC {
		return false, fmt.Errorf("wrong Timezone! Timezone set to: %v", t.Location)
	}
	if t.Hour() >= 20 && t.Minute() >= 01 {
		return true, nil
	}
	return false, nil
}

// AssetMeta represents an assets last update and is ment to be stored in Memcache.
type AssetMeta struct {
	AssetID    string
	LastUpdate time.Time
}

func NewAssetMeta(assetID string) *AssetMeta {
	a := &AssetMeta{
		assetID,
		time.Now()}
	return a
}

// Cache stores the assets metadata with a new timestamp into Memcache.
func (a *AssetMeta) Cache(c appengine.Context) {
	item := &memcache.Item{
		Key:    a.AssetID,
		Object: a,
	}
	memcache.JSON.Set(c, item)
}

func (a *AssetMeta) MsSinceCache(c appengine.Context) int {
	_, err := memcache.JSON.Get(c, a.AssetID, a)
	if err != nil {
		return -1
	}

	age := time.Now().Sub(a.LastUpdate)
	return int(age / time.Millisecond)
}

func timeSiceLastCache(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	for key := range r.URL.Query() {

		a := &AssetMeta{AssetID: key}
		age := a.MsSinceCache(c)
		fmt.Fprintf(w, "%v: Last update %vms ago.\n", key, age)
		if age > 10000 {
			params, err := initiateAsset(c, a.AssetID)
			if err != nil {
				c.Errorf("Errors during inital of asset %s Error: %s", a.AssetID, err)
			}
			scheduleNextUpdate.Call(c, params)
		}
		break
	}
}
