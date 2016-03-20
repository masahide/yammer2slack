package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/bitly/go-simplejson"
	"github.com/masahide/yammer2slack/yammer"
	"github.com/nlopes/slack"
)

const (
	lastFile  = ".lastid.json"
	slackFile = "slack.json"
)

var (
	lsConfig  yammer.LocalServerConfig
	loopNum   = 60 * 60 * 24 * 365 * 5
	sleepTime = 10 * time.Second
	api       = slack.New(key)
	channels  = map[string]*slack.Channel{}
	key       = loadSlackKey(slackFile)
	NameRep   = strings.NewReplacer(
		"(", "",
		")", "",
		".", "",
		"#", "",
		"$", "",
		"@", "",
		"%", "",
		"^", "",
		"&", "",
		"*", "",
		"+", "",
		"=", "",
		"[", "",
		"]", "",
		"{", "",
		"}", "",
		":", "",
		";", "",
		"'", "",
		"<", "",
		">", "",
		"/", "",
		"?", "",
		",", "",
		"|", "",
		"`", "",
		"\"", "",
	)
)

func init() {
	log.SetFlags(log.Lshortfile | log.LstdFlags)
	flag.IntVar(&lsConfig.Port, "p", 8347, "local port: 1024 < ")
	flag.IntVar(&lsConfig.Timeout, "t", 120, "redirect timeout: 0 - 90")
	flag.IntVar(&loopNum, "l", loopNum, "loop count")
	flag.DurationVar(&sleepTime, "s", sleepTime, "sleep time")
	flag.Parse()
}
func getChannels() {
	if len(channels) != 0 {
		return
	}
	chs, err := api.GetChannels(false)
	if err != nil {
		log.Fatal(err)
	}
	for i := range chs {
		channels[chs[i].Name] = &chs[i]
	}
}

func main() {

	var err error

	y := yammer.NewYammer(&lsConfig)
	err = y.YammerAuth()
	if err != nil {
		log.Fatal("Error YammerAuth:", err)
		return
	}
	ticker := make(chan bool)
	go func(ticker chan bool) {
		for i := 1; ; i++ {
			ticker <- true
			if i == loopNum {
				break
			}
			time.Sleep(sleepTime)
		}
		ticker <- false
		close(ticker)
	}(ticker)
	run := make(chan bool, 1)
	run <- false
	i := uint64(1)
	for {
		if !<-ticker {
			break
		}
		select {
		case <-run:
			log.Printf("start getMessage:%d", i)
			i++
			go func(run chan bool) {
				getsAndSends(y)
				run <- false
			}(run)
		default:
		}
	}
	<-run
}

func getsAndSends(y *yammer.Yammer) {
	channels = map[string]*slack.Channel{}
	ids := loadLastid()
	ids.ReceivedId = getAndSend(ids.ReceivedId, y.GetReceived)
	ids.PrivateId = getAndSend(ids.PrivateId, y.GetPrivate)
	saveLastid(ids)
}

func getAndSend(lastId int, getMsgFunc func(int, int) ([]byte, error)) int {
	msgJson, err := getMsgFunc(lastId, 0)
	if err != nil {
		log.Fatal(err)
	}
	messages := getMessages(msgJson)
	if len(messages) != 0 {
		getChannels()
		lastId = messages[0].Id
		for i := len(messages) - 1; i >= 0; i-- {
			postMsg(&messages[i])
		}
	}
	return lastId

}

func loadSlackKey(slackFile string) string {
	m := map[string]string{}
	f, err := os.Open(slackFile)
	if err != nil {
		log.Fatalf("Open %s err:%s", slackFile, err)
	}
	defer f.Close()
	json.NewDecoder(f).Decode(&m)
	k, ok := m["Key"]
	if !ok {
		log.Fatal("load slackFile err: not found 'Key'")
	}
	return k
}
func loadLastid() LastId {
	l := LastId{}
	f, err := os.Open(lastFile)
	if err != nil {
		saveLastid(l)
		return l
	}
	defer f.Close()
	json.NewDecoder(f).Decode(&l)
	return l
}

func saveLastid(ids LastId) {
	f, err := os.Create(lastFile)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	b, err := json.Marshal(ids)
	if err != nil {
		log.Fatal(err)
	}
	if _, err = f.Write(b); err != nil {
		log.Fatal(err)
	}
}

type LastId struct {
	ReceivedId int
	PrivateId  int
}

func getMessages(msgJson []byte) []Msg {
	js, err := simplejson.NewJson(msgJson)
	if err != nil {
		log.Fatal(err)
	}
	refs := js.Get("references")
	users := map[int]User{}
	for i := 0; i < len(refs.MustArray()); i++ {
		ref := refs.GetIndex(i)
		//pp.Print(ref)
		u := User{
			Id:       ref.Get("id").MustInt(),
			Name:     ref.Get("name").MustString(),
			Email:    ref.Get("email").MustString(),
			FullName: ref.Get("full_name").MustString(),
			IconURL:  ref.Get("mugshot_url").MustString(),
		}
		users[u.Id] = u
	}
	msgs := js.Get("messages")
	lenMsg := len(msgs.MustArray())
	messages := make([]Msg, lenMsg)
	for i := 0; i < lenMsg; i++ {
		msg := msgs.GetIndex(i)
		//pp.Print(msg)
		m := Msg{
			Id:        msg.Get("id").MustInt(),
			Body:      msg.Get("body").Get("plain").MustString(),
			Url:       msg.Get("web_url").MustString(),
			CreatedAt: msg.Get("created_at").MustString(),
			ThreadId:  msg.Get("thread_id").MustInt(),
			SenderId:  msg.Get("sender_id").MustInt(),
			Dm:        msg.Get("direct_message").MustBool(),
			GroupId:   msg.Get("group_id").MustInt(),
		}
		if u, ok := users[m.SenderId]; ok {
			m.FullName = u.FullName
			//m.Name = u.Name
			m.IconURL = u.IconURL
		}
		if u, ok := users[m.GroupId]; ok {
			m.GroupName = u.FullName
		}

		messages[i] = m
	}
	return messages
}

func postMsg(m *Msg) {
	chanName := strconv.Itoa(m.ThreadId)
	if m.Dm {
		chanName = "_dm_" + chanName
	} else {
		chanName = m.GroupName + "_" + chanName
	}
	log.Println(chanName)
	var err error
	ch, ok := channels[chanName]
	if !ok {
		ch, err = api.CreateChannel(chanName)
		if err != nil {
			log.Fatalf("CreateChannel:%s err:%s", chanName, err)
		}
		log.Printf("CreateChannel: %s", ch.Name)
		if _, err := api.SetChannelPurpose(ch.ID, m.Url); err != nil {
			log.Fatalf("SetChannelPurpose %s,err:%s", ch.Name, err)
		}
		channels[ch.Name] = ch
	}
	if ch.IsArchived {
		if err = api.UnarchiveChannel(ch.ID); err != nil {
			log.Printf("UnarchiveChannel:%s err %s", ch.Name, err)
		}
		log.Printf("UnarchiveChannel: %s", ch.Name)
	}
	if !ch.IsMember {
		if ch, err = api.JoinChannel(ch.Name); err != nil {
			log.Printf("JoinChannel %s: %s", ch.Name, err)
		}
		channels[ch.Name] = ch
		log.Printf("JoinChannel: %s", ch.Name)
	}
	if ch.Purpose.Value == "" {
		if _, err := api.SetChannelPurpose(ch.ID, m.Url); err != nil {
			log.Fatalf("SetChannelPurpose %s,err:%s", ch.Name, err)
		}
	}
	param := slack.PostMessageParameters{
		Username: strings.TrimSpace(NameRep.Replace(m.FullName)),
		IconURL:  m.IconURL,
	}
	if _, _, err = api.PostMessage(ch.ID, m.Body, param); err != nil {
		log.Printf("err:%s, channel:%s(%s), body:%s, param:%#v", err, ch.ID, ch.Name, m.Body, param)
	}
	log.Printf("PostMessage channel%s, User:%s", ch.Name, m.FullName)
	//pp.Print(m)
}

type User struct {
	Id       int
	Name     string
	Email    string
	FullName string //full_name
	IconURL  string
}

//type references

type Msg struct {
	Id        int
	Body      string
	Url       string
	CreatedAt string
	ThreadId  int
	SenderId  int
	FullName  string
	Name      string
	IconURL   string
	Dm        bool
	GroupId   int //group_id
	GroupName string
}
