package booking

import (
	"encoding/json"
	"fmt"
	"log"
	"regexp"

	"github.com/gocolly/colly"
)

const (
	baseURL  = "http://room.shmtu.edu.cn:8080"
	loginURL = "https://cas.shmtu.edu.cn/cas/login?service=http://room.shmtu.edu.cn:8080/CAS/docs/examples/cas_simple_login.php"
)

// Booker is composed of login info and a colly collector
type Booker struct {
	username  string
	password  string
	lt        string
	execution string
	p         string
	collector *colly.Collector
}

// Room is the status of a bookable room
type Room struct {
	ID            int
	IsValid       bool
	Name          string
	StartTime     string
	EndTime       string
	BookedPeriods [][2]string
	MaxPerson     int
	MinPerson     int
}

// NewBooker creates a new CheckedCollector instance
func NewBooker(username string, password string) (booker *Booker) {
	booker = &Booker{username: username, password: password}
	booker.collector = colly.NewCollector(
		colly.UserAgent(" Mozilla/5.0 (Macintosh; Intel Mac OS X 10_14_4) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/74.0.3729.131 Safari/537.36"))
	booker.getLtAndExecutionValue()
	err := booker.loginAndGetP()
	if err != nil {
		log.Fatal(err)
	}
	booker.collector = booker.collector.Clone()
	booker.collector.Visit(
		fmt.Sprintf("%s/Api/auto_user_check?user=%s&p=%s", baseURL, booker.username, booker.p))
	return
}

func (booker *Booker) getLtAndExecutionValue() {
	booker.collector.OnHTML("input[name]", func(e *colly.HTMLElement) {
		switch e.Attr("name") {
		case "lt":
			booker.lt = e.Attr("value")
		case "execution":
			booker.execution = e.Attr("value")
		default:
			return
		}
		return
	})
	booker.collector.Visit(loginURL)
}

func (booker *Booker) loginAndGetP() (err error) {
	booker.collector.OnResponse(func(r *colly.Response) {
		regP, _ := regexp.Compile(`p=\w+`)
		bodyStr := string(r.Body[:])
		foundP := regP.FindStringSubmatch(bodyStr)
		if foundP == nil {
			log.Fatal("p not found")
		}
		booker.p = foundP[0][2:]
	})
	err = booker.collector.Post(loginURL, map[string]string{
		"username":  booker.username,
		"password":  booker.password,
		"_eventId":  "submit",
		"signin":    "登录",
		"lt":        booker.lt,
		"execution": booker.execution,
	})
	return
}

// BookRoom takes the booking info and book a room in smu library
func (booker *Booker) BookRoom(startTime string, endTime string, day string, title string, application string, teamusers []string, mobile string) (err error) {
	c := booker.collector.Clone()
	requestDataStr := fmt.Sprintf(
		"startTime=%s&endTime=%s&day=%s&title=%s&application=%s&mobile=%s&userid=%s&type=%d&isPublic=%t",
		startTime,
		endTime,
		day,
		title,
		application,
		mobile,
		booker.username,
		2,
		false)
	for _, teamuser := range teamusers {
		requestDataStr += "&teamusers[]=" + teamuser
	}
	for _, cookie := range c.Cookies(baseURL) {
		if cookie.Name == "access_token" {
			requestDataStr += "&access_token=" + cookie.Value
			break
		}
	}
	requestData := []byte(requestDataStr)

	c.OnResponse(func(r *colly.Response) {
		log.Println(string(r.Body[:]))
	})

	err = c.PostRaw(baseURL+"/api.php/spaces/3070/studybook", requestData)
	return
}

// FetchRoomsStatus fetches status of all rooms according to the given date
func (booker *Booker) FetchRoomsStatus(day string) (roomsStatus []*Room) {
	c := booker.collector.Clone()
	var decodedBody map[string]interface{}

	c.OnResponse(func(r *colly.Response) {
		rawBody := r.Body
		err := json.Unmarshal(rawBody, &decodedBody)
		if err != nil {
			log.Fatal(err)
		}
		for _, rawRoomData := range decodedBody["rooms"].([]interface{}) {
			rawData := rawRoomData.(map[string]interface{})
			roomsStatus = append(roomsStatus, newRoom(rawData))
		}
	})
	c.Visit(baseURL + "/api.php/studyinfo/1?day=" + day)
	return
}

func newRoom(rawData map[string]interface{}) (room *Room) {
	room = new(Room)

	rawDetail := rawData["detail"].(map[string]interface{})

	room.ID = int(rawData["id"].(float64))
	room.IsValid = func() bool {
		if rawData["isValid"].(float64) == 1 {
			return true
		}
		return false
	}()
	room.Name = rawData["name"].(string)
	room.StartTime = rawDetail["startTime"].(string)
	room.EndTime = rawDetail["endTime"].(string)
	room.BookedPeriods = func() [][2]string {
		bookBeginTime := rawDetail["bookbegintime"].([]interface{})
		bookEndTime := rawDetail["bookendtime"].([]interface{})
		var bookedPeriods [][2]string
		for i := 0; i < len(bookBeginTime); i++ {
			if bookBeginTime[i] == bookEndTime[i] {
				continue
			}
			bookedPeriods = append(bookedPeriods, [2]string{bookBeginTime[i].(string), bookEndTime[i].(string)})
		}
		return bookedPeriods
	}()
	room.MaxPerson = int(rawDetail["maxPerson"].(float64))
	room.MinPerson = int(rawDetail["minPerson"].(float64))
	return
}