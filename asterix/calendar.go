package asterix

import (
	"bytes"
	"encoding/json"
	"github.com/romitou/previsix/config"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const dayFormat = "2006-01-02"

type calendarItem struct {
	Day   string `json:"day"`
	Times string `json:"times"`
}

type CalendarDay struct {
	OpenDate  time.Time `json:"parkOpeningTime"`
	CloseDate time.Time `json:"parkClosingTime"`
}

type calendarResponse struct {
	Data struct {
		CalendarItems []calendarItem `json:"calendar"`
	} `json:"data"`
}

func fetchCalendar() []calendarItem {
	query, err := GraphQlQuery{
		Query:     config.Get().Asterix.Queries.Calendar,
		Variables: map[string]interface{}{},
	}.ToJSON()
	if err != nil {
		log.Fatal(err)
		return nil
	}

	response, err := http.Post(config.Get().Asterix.Endpoint, "application/json", bytes.NewReader(query))
	if err != nil {
		log.Fatal(err)
		return nil
	}

	var calResponse calendarResponse
	err = json.NewDecoder(response.Body).Decode(&calResponse)
	if err != nil {
		log.Fatal(err)
		return nil
	}

	return calResponse.Data.CalendarItems
}

func GetCalendar() []CalendarDay {
	calendar := fetchCalendar()
	var calendarDays []CalendarDay

	location, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		log.Println(err)
		return nil
	}

	for _, item := range calendar {
		var day time.Time
		day, err = time.Parse(dayFormat, item.Day)
		if err != nil {
			log.Println(err)
			continue
		}
		pairTimes := strings.Split(item.Times, " et ")
		for _, dayTime := range pairTimes {
			times := strings.Split(dayTime, "-")
			if len(times) != 2 {
				log.Println("invalid time format: ", item.Times)
				continue
			}

			times[0] = strings.ReplaceAll(times[0], " ", "")
			times[0] = strings.ReplaceAll(times[0], "H", "h")

			times[1] = strings.ReplaceAll(times[1], " ", "")
			times[1] = strings.ReplaceAll(times[1], "H", "h")

			var openingHour int
			opening := strings.Split(times[0], "h")
			if len(opening) > 0 {
				openingHour, err = strconv.Atoi(opening[0])
				if err != nil {
					log.Println(err)
					continue
				}
			}

			var closingHour int
			closing := strings.Split(times[1], "h")
			if len(closing) > 0 {
				closingHour, err = strconv.Atoi(closing[0])
				if err != nil {
					log.Println(err)
					continue
				}
			}

			openingTime := time.Date(day.Year(), day.Month(), day.Day(), openingHour, 0, 0, 0, location)
			closingTime := time.Date(day.Year(), day.Month(), day.Day(), closingHour, 0, 0, 0, location)

			// if closing time is before opening time, it means the park closes the next day, right?
			if closingTime.Before(openingTime) {
				closingTime = closingTime.AddDate(0, 0, 1)
			}

			calendarDays = append(calendarDays, CalendarDay{
				OpenDate:  openingTime,
				CloseDate: closingTime,
			})
		}
	}
	return calendarDays
}
