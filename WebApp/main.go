package main

import (
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"sync"
	"net/http"
	"regexp"
	"strings"
	"time"
//	"encoding/json"
	"encoding/base64"
	"crypto/rand"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
)

func init() {
	config = &oauth2.Config{
		RedirectURL:  host + "/callback",
		ClientID:     GOOGLE_CLIENT_ID,
		ClientSecret: GOOGLE_CLIENT_SECRET,
		Scopes:       []string{"https://www.googleapis.com/auth/calendar"},
		Endpoint:     google.Endpoint,
	}
}

func (h *Handler) handlerMain(w http.ResponseWriter, r *http.Request) {
	var htmlIndex = `
	<html>
	<head>
	<style>
	.block1{
		position: fixed;
		top: 50%;
		width: 200px;
		height: 100px;
		left: 25%;
	}
	</style>
	</head>
	<body>
		<div align="center">
		<p>
		Данное web-приложение позволяет загрузить расписание любой группы физфака МГУ в Google-Calendar. Программа работает в сыром режиме, возможны баги etc. Перед началом работы необходимо авторизироваться(кнопка ниже). Затем будет предложено выбрать группу из списка. Ожидание выгрузки составляет от 10 до 30 секунд. Наберитесь терпения.
		</p>
		</div>
		<div align="center">
		<a href="/login">Google Log In</a>
		</div>
	</body>
	</html>`
	fmt.Fprintf(w, htmlIndex)
}

var mu = &sync.Mutex{}

func (h *Handler) handlerGoogleLogin(w http.ResponseWriter, r *http.Request) {
	//TODO
	size := 16
	rb := make([]byte, size)
	_, err := rand.Read(rb)
	if err != nil {
		log.Fatal(err)
	}
	oauthStateString := base64.URLEncoding.EncodeToString(rb)
	h.State = oauthStateString
	url := config.AuthCodeURL(oauthStateString, oauth2.AccessTypeOffline)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func (h *Handler) handlerGoogleCallback(w http.ResponseWriter, r *http.Request) {
	size := 16
	rb := make([]byte, size)
	_, err := rand.Read(rb)
	if err != nil {
		log.Fatal(err)
	}
	
	c, err := r.Cookie("session")
	code := ""
	
	if err != nil {
		fmt.Println("new cookie")
		oauthStateString := base64.URLEncoding.EncodeToString(rb)
		cook := &http.Cookie{
				Name: "session",
				Value: oauthStateString,
				Expires: time.Now().AddDate(0,0,1),
				Path: host + "/result",
			}
		http.SetCookie(w, cook)
		code = r.FormValue("code")
		mu.Lock()
		st := h.Sessions[cook.Value]
		st.Code = code
		h.Sessions[cook.Value] = st
		r.AddCookie(cook)
		mu.Unlock()
	} else {
		if _, ok := h.Sessions[c.Value]; ok{
			code = h.Sessions[c.Value].Code
		} else{
			fmt.Println("DELETE", c)
			http.SetCookie(w, &http.Cookie{
				Name: c.Name,
				MaxAge: -1,
				Expires: time.Now().Add(-100 * time.Minute),
			})
			http.Redirect(w, r, host + "/", http.StatusTemporaryRedirect)
			return
		}
	}
	fmt.Println(h.Sessions, c)

	for _, c := range r.Cookies(){
		fmt.Println("COOKIES", c)
	}
	
	tmpl, err := template.ParseGlob("index.html")
	if err != nil {
		log.Fatal(err)
	}
	tmpl.ExecuteTemplate(w, "index.html", struct{}{})
}

func (h *Handler) handlerResult(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	c, err := r.Cookie("session")
	if err != nil {
		fmt.Fprintf(w, "no cookie")
		return
	}
	group := r.FormValue("group")
	fmt.Println("RES: ", h.Sessions)
	for _, c := range r.Cookies(){
		fmt.Println("res cookies", c)
	}
	
	if ses, ok := h.Sessions[c.Value]; ok{
		fmt.Println("code", ses.Code)
		if ses.Code == ""{
			http.Redirect(w, r, host + "/login", http.StatusTemporaryRedirect)
			mu.Unlock()
			return
		}
		fmt.Println(h.Sessions)
		client := &http.Client{}
		fmt.Println(h.Sessions[c.Value].Client)
		if h.Sessions[c.Value].Client == nil {
			client, err = getClient(ses.Code)
			if err != nil {
				log.Fatal(err)
			}
			st := h.Sessions[c.Value]
			st.Client = client
			h.Sessions[c.Value] = st
		} else {
			client = h.Sessions[c.Value].Client
		}
		mu.Unlock()
		
		go putData(client, group)
		http.Redirect(w, r, urlCalendar, http.StatusTemporaryRedirect)
		return
	}
	mu.Unlock()
	http.Redirect(w, r, host + "/callback", http.StatusTemporaryRedirect)
}

func getClient(code string) (*http.Client, error) {
	client := &http.Client{}
	token, err := config.Exchange(oauth2.NoContext, code)
	if err != nil {
		return client, fmt.Errorf("code exchange failed: %s", err.Error())
	}
	client = config.Client(oauth2.NoContext, token)

	return client, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/":
		h.handlerMain(w, r)
	case "/login":
		h.handlerGoogleLogin(w, r)
	case "/callback":
		h.handlerGoogleCallback(w, r)
	case "/result":
		h.handlerResult(w, r)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func main() {
	var sessions = make(map[string]Session)

	handler := &Handler{
		Sessions: sessions,
		State: "LOL",
	}
	port := "8080"
	fmt.Println("starting server at :" + port)
	http.ListenAndServe(":"+port, handler)
}

func putData(client *http.Client, group string) {
	srv, err := calendar.New(client)
	if err != nil {
		log.Fatalf("Unable to retrieve Calendar client: %v", err)
	}

	// ====================================================================
	// Get data from database
	db, err := sql.Open("mysql", DSN)
	if err != nil {
		log.Fatal(err)
	}
	err = db.Ping()
	if err != nil {
		log.Fatal(err)
	}

	allWeek := dbExplorer(db, group)

	clndr := &calendar.Calendar{
		Summary: "Shedule" + group,
	}
	insertedCalendar, err := srv.Calendars.Insert(clndr).Do()

	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("==========")
	//	calendarId := "primary" // Use account shedule
	calendarId := insertedCalendar.Id

	var isOdd bool
	var day int
	var endSemester string

	var endSummer = "0601"
	var endWinter = "1231"

	timeNow := time.Now()
	year := timeNow.Year()
	month := int(timeNow.Month())

	if month > 0 && month < 8 {
		month = 2
		day = 7
		endSemester = fmt.Sprintf("%v", year) + endSummer
	} else if month > 7 && month < 13 {
		month = 9
		day = 1
		endSemester = fmt.Sprintf("%v", year) + endWinter
	}

	t := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	firstDay := int(t.Weekday()) - 1
	t = t.AddDate(0, 0, 7-firstDay)

	for j := 0; j < len(allWeek); j++ {
		if j == firstDay {
			isOdd = !isOdd
			t = t.AddDate(0, 0, -7)
		}
		lessonStart := t.Format("2006-01-02")
		for i, lesson := range allWeek[j] {
			st := DataToParsingAt{
				Lesson:      lesson,
				Number:      i,
				Parity:      isOdd,
				StartTime:   lessonStart,
				Time:        t,
				SemesterEnd: endSemester,
			}
			events, isEmpty := st.parseAt()
			if isEmpty {
				continue
			}
			for _, event := range events {
				event, err = srv.Events.Insert(calendarId, event).Do()
				if err != nil {
					log.Fatalf("Unable to create event. %v\n", err)
				}
				fmt.Printf("Event created: %s\n", event.HtmlLink)
			}
		}
		t = t.AddDate(0, 0, 1)
	}
}

func (st *Subject) parseSharp() []Subject {
	count := strings.Count(st.Name, "#")
	str := strings.Repeat("(.*)#", count) + "(.*)"
	reSharp := regexp.MustCompile(str)

	names := reSharp.FindStringSubmatch(st.Name)[1 : count+2]
	lectors := reSharp.FindStringSubmatch(st.Lector)[1 : count+2]
	rooms := reSharp.FindStringSubmatch(st.Room)[1 : count+2]

	var subjects = make([]Subject, 0, 1)
	for i := 0; i < len(names); i++ {
		subj := Subject{
			Name:   names[i],
			Room:   rooms[i],
			Lector: lectors[i],
			Parity: st.Parity,
		}
		subjects = append(subjects, subj)
	}
	return subjects
}

func (st *DataToParsingAt) parseAt() ([]*calendar.Event, bool) {
	subject := st.Lesson
	isOdd := st.Parity
	lessonStart := st.StartTime
	t := st.Time

	var result = make([]*calendar.Event, 0, 2)
	if subject.Name == "" || subject.Name == "__" {
		return result, true
	}

	if strings.Contains(cases, subject.Name) {
		return result, true
	}

	if strings.Contains(subject.Name, "@") {
		st.IsAllDay = false

		regName := reAt.FindStringSubmatch(subject.Name)
		regLector := reAt.FindStringSubmatch(subject.Lector)
		regRoom := reAt.FindStringSubmatch(subject.Room)

		var oddLessonStart string
		var evenLessonStart string

		if isOdd {
			oddLessonStart = lessonStart
			evenLessonStart = t.AddDate(0, 0, 7).Format("2006-01-02")
		} else {
			oddLessonStart = t.AddDate(0, 0, 7).Format("2006-01-02")
			evenLessonStart = lessonStart
		}

		oddSubject := Subject{Name: regName[1], Lector: regLector[1], Room: regRoom[1], Parity: oddLessonStart}
		evenSubject := Subject{Name: regName[2], Lector: regLector[2], Room: regRoom[2], Parity: evenLessonStart}

		var arr = []Subject{oddSubject, evenSubject}
		for _, subj := range arr {
			var fromSharp = make([]Subject, 0, 1)
			if strings.Contains(subj.Name, "#") {
				fromSharp = subj.parseSharp()
			} else {
				fromSharp = append(fromSharp, subj)
			}
			for _, v := range fromSharp {
				if v.Name != "" && v.Name != "__" && v.Name != practice {
					st.StartTime = v.Parity
					st.Lesson = v
					event := st.createEvent()
					result = append(result, event)
				}
			}
		}
		return result, false
	}

	var fromSharp = make([]Subject, 0, 1)
	if subject.Name == practice {
		return result, true
	}
	if strings.Contains(subject.Name, "#") {
		fromSharp = subject.parseSharp()
	} else {
		fromSharp = append(fromSharp, subject)
	}
	st.IsAllDay = true
	for _, v := range fromSharp {
		st.Lesson = v
		event := st.createEvent()
		result = append(result, event)
	}

	return result, false
}

func (st *DataToParsingAt) createEvent() *calendar.Event {
	subject := st.Lesson
	i := st.Number
	allDay := st.IsAllDay
	lessonStart := st.StartTime
	endSemester := st.SemesterEnd

	var freq = make([]string, 0, 1)
	if allDay {
		freq = []string{"RRULE:FREQ=WEEKLY;UNTIL=" + endSemester}
	} else {
		freq = []string{"RRULE:FREQ=WEEKLY;INTERVAL=2;UNTIL=" + endSemester}
	}
	color := getColorId(subject.Name, subject.Room)
	if subject.Lector == "__" {
		subject.Lector = ""
	}
	if subject.Room == "__" {
		subject.Room = ""
	}
	if _, isNorth := north[subject.Room]; isNorth {
		subject.Room = subject.Room + "(СЕВЕР)"
	}
	if _, isSouth := south[subject.Room]; isSouth {
		subject.Room = subject.Room + "(ЮГ)"
	}
	event := &calendar.Event{
		Summary:     subject.Room + " " + subject.Name + " " + subject.Lector,
		Location:    "Lomonosov Moscow State University", //Number of room and direction?
		Description: subject.Lector,
		Start: &calendar.EventDateTime{
			DateTime: lessonStart + timeIntervals[i].Start, // spring ----> season
			TimeZone: "Europe/Moscow",
		},
		End: &calendar.EventDateTime{
			DateTime: lessonStart + timeIntervals[i].End,
			TimeZone: "Europe/Moscow",
		},
		ColorId: color,
		Reminders: &calendar.EventReminders{
			UseDefault: false,
			Overrides:  []*calendar.EventReminder{},
			//ForceSendFields is required, if you dont want to set up notifications, because
			//by default, empty values are omitted from API requests
			ForceSendFields: []string{"UseDefault", "Overrides"},
		},
		Recurrence: freq,
	}
	return event
}

func getColorId(name, room string) string {
	/*
		ColorId : Color
		1 : lavender
		2 : sage //шалфей
		3 : grape
		4 : flamingo
		5 : banana
		6 : mandarin
		7 : peacock //павлин
		8 : graphite
		9 : blueberry
		10 : basil //базилик
		11 : tomato
	*/
	if name == war {
		return "11"
	} else if name == practice {
		return "10"
	} else if name == mfk || name == MFKabbr || name == MFK {
		return "4"
	}
	_, isLecture := audience[room]
	if reUpp.MatchString(name) || isLecture {
		return "3"
	}
	if strings.Contains(name, "с/к") || strings.Contains(name, "НИС") || strings.Contains(name, "ДМП") || strings.Contains(name, "Д/п") || strings.Contains(name, "Д/П") || strings.Contains(name, "C/К") || strings.Contains(name, "С/К") || strings.Contains(name, "ФТД") {
		return "2"
	}
	return "7"
}
