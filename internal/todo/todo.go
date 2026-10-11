package todo

import "time"

type Task struct {
	Title string
	Notes string
	Due   time.Time
}
