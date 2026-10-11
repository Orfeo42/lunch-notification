package meal

import (
	"strings"
	"time"
)

type sentinelError string

func (e sentinelError) Error() string { return string(e) }

const ErrUnknownKind sentinelError = "unknown meal kind"

type Kind string

const (
	Lunch  Kind = "lunch"
	Dinner Kind = "dinner"
)

type Meal struct {
	Kind Kind
	Dish string
}

type DayPlan struct {
	Day   time.Time
	Meals []Meal
}

func ParseKind(label string) (Kind, error) {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "pranzo":
		return Lunch, nil
	case "cena":
		return Dinner, nil
	default:
		return "", ErrUnknownKind
	}
}

func (k Kind) Label() string {
	switch k {
	case Lunch:
		return "Pranzo"
	case Dinner:
		return "Cena"
	default:
		return string(k)
	}
}
