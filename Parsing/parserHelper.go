package main

import (
	"database/sql"
	"fmt"
	"fopSchedule/common"
	"log"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

func (st *DataToParsingLine) parseLine(
	subjectIndex, countSmall0 int, text string, nextLine, is2Weeks, isFirstInSmall0 bool,
) ([]Department, []string, error) {
	departments := st.Departments
	allGr := st.AllGroups
	resFromReg := st.ResultFromReqexp
	insertedGroups := st.InsertedGroups
	subject := st.Lesson
	reInterval := st.RegexpInterval

	if len(resFromReg) == 0 {
		for _, dep := range departments {
			for _, gr := range allGr {
				if dep.Number != gr {
					continue
				}

				if !nextLine {
					if countSmall0 < 0 {
						dep.Lessons[subjectIndex] = subject
						continue
					}

					newSubj := common.Subject{}
					if isFirstInSmall0 {
						newSubj = subject
					} else {
						newSubj = subject.GetNewStruct(dep.Lessons[subjectIndex], "#")
					}

					dep.Lessons[subjectIndex] = newSubj
					continue
				}

				// new part
				if !strings.Contains(dep.Lessons[subjectIndex].Name, "@") {
					newSubj := subject.GetNewStruct(dep.Lessons[subjectIndex], "@")
					dep.Lessons[subjectIndex] = newSubj
					insertedGroups = append(insertedGroups, gr)
					continue
				}

				regName := strings.Split(dep.Lessons[subjectIndex].Name, "@")[1]
				regLector := strings.Split(dep.Lessons[subjectIndex].Lector, "@")[1]
				regRoom := strings.Split(dep.Lessons[subjectIndex].Room, "@")[1]

				/*
					regName := reAt.FindStringSubmatch(dep.Lessons[subjectIndex].Name)[2]
					regLector := reAt.FindStringSubmatch(dep.Lessons[subjectIndex].Lector)[2]
					regRoom := reAt.FindStringSubmatch(dep.Lessons[subjectIndex].Room)[2]
				*/

				if subject.Name != regName || subject.Room != regRoom || subject.Lector != regLector {
					newSubj := subject.GetNewStruct(dep.Lessons[subjectIndex], "#")
					dep.Lessons[subjectIndex] = newSubj
				}
				insertedGroups = append(insertedGroups, gr)
				// end of new part

				// newSubj := subject.getNewStruct(dep.Lessons[subjectIndex], "@")
				// dep.Lessons[subjectIndex] = newSubj
			}
		}

		return departments, insertedGroups, nil
	}

	if reInterval.MatchString(text) {
		interval := reInterval.FindStringSubmatch(text)
		left, err := strconv.Atoi(interval[1])
		if err != nil {
			return nil, nil, err
		}

		right, err := strconv.Atoi(interval[2])
		if err != nil {
			return nil, nil, err
		}

		for i := left + 1; i < right; i++ {
			resFromReg = append(resFromReg, strconv.Itoa(i))
		}
	}

	for _, gr := range resFromReg {
		if _, ok := common.SubGroups[gr]; ok {
			resFromReg = append(resFromReg, common.SubGroups[gr]...)
		}
	}

	for _, dep := range departments {
		for _, gr := range resFromReg {
			if dep.Number != gr {
				continue
			}
			
			if !nextLine {
				if dep.Lessons[subjectIndex].Name == "" {
					dep.Lessons[subjectIndex] = subject
					insertedGroups = append(insertedGroups, gr)
					continue
				}
				var subjs = make([]common.Subject, 0, 1)
				if strings.Contains(dep.Lessons[subjectIndex].Name, "#") {
					subjs = dep.Lessons[subjectIndex].ParseSharp()
				} else {
					subjs = append(subjs, dep.Lessons[subjectIndex])
				}

				var isNewSubject = true
				for _, s := range subjs {
					if subject.Name == s.Name && subject.Room == s.Room && subject.Lector == s.Lector {
						isNewSubject = false
						break
					}
				}
				if isNewSubject {
					newSubj := subject.GetNewStruct(dep.Lessons[subjectIndex], "#")
					dep.Lessons[subjectIndex] = newSubj
				}

				insertedGroups = append(insertedGroups, gr)
				continue
			}

			if !strings.Contains(dep.Lessons[subjectIndex].Name, "@") {
				newSubj := subject.GetNewStruct(dep.Lessons[subjectIndex], "@")
				dep.Lessons[subjectIndex] = newSubj
				insertedGroups = append(insertedGroups, gr)
				continue
			}

			regName := strings.Split(dep.Lessons[subjectIndex].Name, "@")[1]
			regLector := strings.Split(dep.Lessons[subjectIndex].Lector, "@")[1]
			regRoom := strings.Split(dep.Lessons[subjectIndex].Room, "@")[1]

			/*
				regName := reAt.FindStringSubmatch(dep.Lessons[subjectIndex].Name)[2]
				regLector := reAt.FindStringSubmatch(dep.Lessons[subjectIndex].Lector)[2]
				regRoom := reAt.FindStringSubmatch(dep.Lessons[subjectIndex].Room)[2]
			*/

			if subject.Name != regName || subject.Room != regRoom || subject.Lector != regLector {
				newSubj := subject.GetNewStruct(dep.Lessons[subjectIndex], "#")
				dep.Lessons[subjectIndex] = newSubj
			}
			insertedGroups = append(insertedGroups, gr)
		}
	}

	if countSmall0 > 0 || is2Weeks {
		return departments, insertedGroups, nil
	}

	var mapAllGr = make(map[string]string)
	for _, gr := range allGr {
		mapAllGr[gr] = ""
	}

	for _, v1 := range insertedGroups {
		for v2, _ := range mapAllGr {
			if v2 == v1 {
				delete(mapAllGr, v2)
			}
		}
	}

	for _, dep := range departments {
		for gr, _ := range mapAllGr {
			if dep.Number != gr {
				continue
			}

			newSubj := common.Subject{
				Name:   "__",
				Lector: "__",
				Room:   "__",
			}

			if nextLine {
				newSubj = newSubj.GetNewStruct(dep.Lessons[subjectIndex], "@")
			}

			dep.Lessons[subjectIndex] = newSubj
		}
	}

	if countSmall0 <= 0 {
		insertedGroups = make([]string, 5)
	}

	return departments, insertedGroups, nil
}

func putToDB(departments []Department, db *sql.DB) {
	for _, val := range departments {
		var valuesToDB = make([]interface{}, 0, 1)
		for _, les := range val.Lessons {
			value := les.Name + "%" + les.Lector + "%" + les.Room
			valuesToDB = append(valuesToDB, value)
		}
		//		fmt.Println(valuesToDB)
		req := fmt.Sprintf("INSERT INTO `%v`"+common.Columns+"VALUES"+common.QuesStr, val.Number)
		statement, err := db.Prepare(req)
		if err != nil {
			log.Fatal(err)
		}
		_, err = statement.Exec(valuesToDB...)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(val.Number)
		fmt.Println("PUT IN TABLE")
	}
}

func clean(arr []Department) []Department {
	var result = make([]Department, len(arr))
	for i, d := range arr {
		s := Department{}
		s.Number = d.Number
		s.Lessons = make([]common.Subject, len(d.Lessons))
		result[i] = s
	}

	return result
}

func fromStringToInt(class string) int {
	num := common.ReNum.FindStringSubmatch(class)[1]
	numberFromClass, err := strconv.Atoi(num)
	if err != nil {
		log.Fatal(class, err)
	}

	return numberFromClass
}

func parseGroups(text, room string) common.Subject {
	subj := common.Subject{}

	var isSpace bool
	for _, val := range text {
		isSpace = unicode.IsSpace(val)
		break
	}

	if isSpace {
		subj.Name = "__"
		subj.Room = "__"
		subj.Lector = "__"

		return subj
	}

	for lesson, _ := range common.LessonMap {
		if strings.Contains(text, lesson) {
			subj.Name = lesson
			subj.Room = "__"
			subj.Lector = "__"

			return subj
		}
	}

	// TODO: Egor, FIX IT
	// ex.: 429 - С/К по выбору доц. Водовозов В. Ю.
	if len(room) == 0 {
		subj.Name = text
		subj.Room = "__"
		subj.Lector = "__"

		return subj
	}

	fmt.Printf("ROOM: %s, TEXT: %s\n", room, text)
	rLect := regexp.MustCompile(`.* ` + room + ` (.*)`)
	Lect := rLect.FindStringSubmatch(text)[1]

	var rSubj *regexp.Regexp
	if common.ReDash.MatchString(text) {
		rSubj = regexp.MustCompile(`([^0-9\-]+) ` + room + " " + Lect)
	} else {
		rSubj = regexp.MustCompile(`([^0-9]+) ` + room + " " + Lect)
	}
	Subj := rSubj.FindStringSubmatch(text)[1]

	subj.Name = Subj
	subj.Lector = Lect
	subj.Room = room

	return subj
}
