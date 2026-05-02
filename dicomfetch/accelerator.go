package dicomfetch

import (
	"errors"
	"fmt"
	"time"

	"github.com/neomat-prog/go-dicom-gateway/source"
)

type Options struct {
	MaxConcurrency int
	BatchSize      int
	RequestTimeout time.Duration
}

type Fetcher struct {
	Source  source.Source
	Options Options
}

/*
	WindowBehind = how many previous instances to warm
	WindowAhead = how many next instances to prefetch
*/

type Window struct {
	WindowAhead  int
	WindowBehind int
}

/*
Example usage:
	refs:   0 1 2 3 4 5 6 7 8 9
	center: 4
	behind: 2
	ahead:  3

	result: 2 3 4 5 6 7
*/

func SelectWindow(refs []source.InstanceRef, center int, behind int, ahead int) ([]source.InstanceRef, error) {

	if len(refs) == 0 {
		return nil, errors.New("refs cannot be empty")
	}

	if center < 0 || center >= len(refs) {
		return nil, fmt.Errorf("center index %d out of range [0,%d)", center, len(refs))
	}

	if behind < 0 {
		return nil, errors.New("behind cannot be negative")
	}

	if ahead < 0 {
		return nil, errors.New("ahead cannot be negative")
	}

	start := center - behind

	if start < 0 {
		start = 0
	}

	end := center + ahead + 1

	if end > len(refs) {
		end = len(refs)
	}

	return refs[start:end], nil
}
