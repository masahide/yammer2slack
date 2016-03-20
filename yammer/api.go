package yammer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strconv"
)

const (
	by_emailURL  = "https://www.yammer.com/api/v1/users/by_email.json" // ?email=user@domain.com
	postURL      = "https://www.yammer.com/api/v1/messages.json"       // body replied_to_id
	requestURL   = "https://www.yammer.com/api/v1/messages.json"
	inboxURL     = "https://www.yammer.com/api/v1/messages/inbox.json" // ?newer_than=[:message_id]
	followingURL = "https://www.yammer.com/api/v1/messages/following.json"
	ReceivedURL  = "https://www.yammer.com/api/v1/messages/received.json"
	privateURL   = "https://www.yammer.com/api/v1/messages/private.json"
)

func (y *Yammer) EmailToIDYammer(email string) (id int, err error) {
	r, in_err := y.transport.Client().Get(by_emailURL + "?email=" + email)
	if in_err != nil {
		log.Fatal("Get:", in_err)
		return 0, in_err
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		log.Fatalf("updateToken %v", r.Status)
		return
	}
	var data interface{}
	if err = json.NewDecoder(r.Body).Decode(&data); err != nil {
		return
	}
	id = int(data.([]interface{})[0].(map[string]interface{})["id"].(float64))
	fmt.Printf("--- id:\n%# v\n\n", id)
	return
}

func (y *Yammer) GetPrivate(last_id, limit int) ([]byte, error) {
	return y.getMessage(privateURL, last_id, limit)
}
func (y *Yammer) GetInbox(last_id, limit int) ([]byte, error) {
	return y.getMessage(inboxURL, last_id, limit)
}
func (y *Yammer) GetFollowing(last_id, limit int) ([]byte, error) {
	return y.getMessage(followingURL, last_id, limit)
}
func (y *Yammer) GetReceived(last_id, limit int) ([]byte, error) {
	return y.getMessage(ReceivedURL, last_id, limit)
}

func (y *Yammer) getMessage(endpoint string, last_id, limit int) ([]byte, error) {
	urlValues := url.Values{}

	if last_id != 0 {
		urlValues.Add("newer_than", strconv.Itoa(last_id))
	}
	if limit != 0 {
		urlValues.Add("limit", strconv.Itoa(limit))
	}
	//pp.Print(endpoint + "?" + urlValues.Encode())
	r, in_err := y.transport.Client().Get(endpoint + "?" + urlValues.Encode())
	if in_err != nil {
		log.Fatalf("Get:%s", in_err)
		return nil, in_err
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		log.Fatalf("updateToken %v", r.Status)
	}
	return ioutil.ReadAll(r.Body)
}

func (y *Yammer) Send(method string, id int, message string) (string, error) {

	r, err := y.transport.Client().PostForm(postURL, url.Values{
		method: {strconv.Itoa(id)},
		"body": {message},
	})
	if err != nil {
		log.Fatalf("Get:%s", err)
		return "", err
	}
	defer r.Body.Close()
	buf := new(bytes.Buffer)
	buf.ReadFrom(r.Body)
	if r.StatusCode != 200 {
		return buf.String(), fmt.Errorf("sendMessage Code:%d, Status:%v", r.StatusCode, r.Status)
	}
	return buf.String(), nil
}

func (y *Yammer) Unfollow(id string) (string, error) {

	req, err := http.NewRequest("DELETE", "https://www.yammer.com/api/v1/threads/"+id+"/follow.json", nil)
	if err != nil {
		log.Fatalf("NewRequest:%s", err)
		return "", err
	}
	r, err := y.transport.Client().Do(req)
	if err != nil {
		log.Fatalf("Get:%s", err)
		return "", err
	}
	defer r.Body.Close()
	buf := new(bytes.Buffer)
	buf.ReadFrom(r.Body)
	if r.StatusCode != 200 {
		return buf.String(), fmt.Errorf("Unsubscribe Code:%d, Status:%v", r.StatusCode, r.Status)
	}
	return buf.String(), nil
}
