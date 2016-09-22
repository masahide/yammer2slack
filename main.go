package main

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"flag"
	"io"
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
	lastFile  = "lastid.json"
	slackFile = "slack.json"
)

var (
	lsConfig  yammer.LocalServerConfig
	loopNum   = 1
	sleepTime = 120 * time.Second
	api       = slack.New(key)
	channels  = map[string]*slack.Channel{}
	key       = loadSlackKey(slackFile)
	nameRep   = strings.NewReplacer(
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
func getChannels() error {
	if len(channels) != 0 {
		return nil
	}
	chs, err := api.GetChannels(false)
	if err != nil {
		log.Println(err)
		return err
	}
	for i := range chs {
		channels[chs[i].Name] = &chs[i]
	}
	return nil
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
			if loopNum != 1 {
				log.Printf("start getMessage:%d", i)
			}
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
	ids.ReceivedID = getAndSend(ids.ReceivedID, y.GetReceived)
	ids.PrivateID = getAndSend(ids.PrivateID, y.GetPrivate)
	saveLastid(ids)
}

func getAndSend(lastID int, getMsgFunc func(int, int) ([]byte, error)) int {
	msgJSON, err := getMsgFunc(lastID, 0)
	if err != nil {
		log.Println(err)
		return lastID
	}
	messages := getMessages(msgJSON)
	if len(messages) != 0 {
		if err := getChannels(); err != nil {
			return lastID
		}
		for i := len(messages) - 1; i >= 0; i-- {
			if err := postMsg(&messages[i]); err != nil {
				return lastID
			}
		}
		lastID = messages[0].id
	}
	return lastID

}

func printClose(c io.Closer) {
	if err := c.Close(); err != nil {
		log.Println(err)
	}
}

func loadSlackKey(slackFile string) string {
	m := map[string]string{}
	f, err := os.Open(slackFile)
	if err != nil {
		log.Fatalf("Open %s err:%s", slackFile, err)
	}
	defer printClose(f)
	if err = json.NewDecoder(f).Decode(&m); err != nil {
		log.Fatalln(err)
	}
	k, ok := m["Key"]
	if !ok {
		log.Fatal("load slackFile err: not found 'Key'")
	}
	return k
}
func loadLastid() LastID {
	l := LastID{}
	f, err := os.Open(lastFile)
	if err != nil {
		saveLastid(l)
		return l
	}
	defer printClose(f)
	if err = json.NewDecoder(f).Decode(&l); err != nil {
		log.Fatalln(err)
	}
	return l
}

func saveLastid(ids LastID) {
	f, err := os.Create(lastFile)
	if err != nil {
		log.Fatal(err)
	}
	defer printClose(f)
	b, err := json.Marshal(ids)
	if err != nil {
		log.Fatal(err)
	}
	if _, err = f.Write(b); err != nil {
		log.Fatal(err)
	}
}

// LastID save file
type LastID struct {
	// ReceivedID  received ID
	ReceivedID int
	// PrivateID  private ID
	PrivateID int
}

func getMessages(msgJSON []byte) []msg {
	js, err := simplejson.NewJson(msgJSON)
	if err != nil {
		log.Println(err)
		return []msg{}
	}
	refs := js.Get("references")
	users := map[int]user{}
	for i := 0; i < len(refs.MustArray()); i++ {
		ref := refs.GetIndex(i)
		//pp.Print(ref)
		u := user{
			id:       ref.Get("id").MustInt(),
			name:     ref.Get("name").MustString(),
			email:    ref.Get("email").MustString(),
			fullName: ref.Get("full_name").MustString(),
			iconURL:  ref.Get("mugshot_url").MustString(),
		}
		users[u.id] = u
	}
	msgs := js.Get("messages")
	lenMsg := len(msgs.MustArray())
	messages := make([]msg, lenMsg)
	for i := 0; i < lenMsg; i++ {
		resMsg := msgs.GetIndex(i)
		//pp.Print(msg)
		m := msg{
			id:        resMsg.Get("id").MustInt(),
			body:      resMsg.Get("body").Get("plain").MustString(),
			url:       resMsg.Get("web_url").MustString(),
			createdAt: resMsg.Get("created_at").MustString(),
			threadID:  resMsg.Get("thread_id").MustInt(),
			senderID:  resMsg.Get("sender_id").MustInt(),
			dm:        resMsg.Get("direct_message").MustBool(),
			groupID:   resMsg.Get("group_id").MustInt(),
		}
		if u, ok := users[m.senderID]; ok {
			m.fullName = u.fullName
			m.name = u.name
			m.iconURL = u.iconURL
		}
		if u, ok := users[m.groupID]; ok {
			m.groupName = u.fullName
		}

		messages[i] = m
	}
	return messages
}

func nameHash(name string) string {
	if len(name) < 21 {
		return name
	}
	hasher := md5.New()
	hasher.Write([]byte(name))
	h := base64.StdEncoding.EncodeToString(hasher.Sum(nil))
	return name[0:15] + strings.ToLower(h[0:6])
}

func makeChannelName(m *msg) string {
	chanName := strconv.Itoa(m.threadID)
	if m.dm {
		chanName = "_dm_" + chanName
	} else {
		chanName = nameHash(m.groupName + "_" + chanName)
	}
	log.Println(chanName)
	return chanName
}

func createChannel(m *msg, chanName string) (ch *slack.Channel, err error) {
	ch, err = api.CreateChannel(chanName)
	if err != nil {
		log.Printf("CreateChannel:%s err:%s", chanName, err)
		return
	}
	log.Printf("CreateChannel: %s", ch.Name)
	if ch.Purpose.Value, err = api.SetChannelPurpose(ch.ID, m.url); err != nil {
		log.Printf("SetChannelPurpose %s,err:%s", ch.Name, err)
		return
	}
	return
}

func postMsg(m *msg) error {
	var err error
	chanName := makeChannelName(m)
	ch, ok := channels[chanName]
	if !ok {
		if ch, err = createChannel(m, chanName); err != nil {
			return err
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
		if _, apierr := api.SetChannelPurpose(ch.ID, m.url); apierr != nil {
			log.Printf("SetChannelPurpose %s,err:%s", ch.Name, apierr)
			return apierr
		}
	}
	param := slack.PostMessageParameters{
		Username: strings.TrimSpace(nameRep.Replace(m.fullName)),
		IconURL:  m.iconURL,
	}
	if _, _, err = api.PostMessage(ch.ID, m.body, param); err != nil {
		log.Printf("err:%s, channel:%s(%s), body:%s, param:%#v", err, ch.ID, ch.Name, m.body, param)
	}
	log.Printf("PostMessage channel%s, user:%s", ch.Name, m.fullName)
	//pp.Print(m)
	return nil
}

type user struct {
	id       int
	name     string
	email    string
	fullName string //full_name
	iconURL  string
}

//type references

type msg struct {
	id        int
	body      string
	url       string
	createdAt string
	threadID  int
	senderID  int
	fullName  string
	name      string
	iconURL   string
	dm        bool
	groupID   int //group_id
	groupName string
}
