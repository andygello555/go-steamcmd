package steamcmd

import (
	"fmt"
	"github.com/andygello555/agem"
	"github.com/pkg/errors"
	"time"
)

// SteamDateLayout represents a format that a release date (or other date) can be in.
type SteamDateLayout string

const (
	DayShortMonthYear         = "2 Jan, 2006"
	DayShortMonthYearNoCommas = "2 Jan 2006"
	ShortMonthDayYear         = "Jan 2, 2006"
	DayShortMonthYearDots     = "2. Jan. 2006"
	// MonthDayNdOrdYear and the 3 subsequent SteamDateLayout(s) all parse the same dates with different ordinal chars.
	MonthDayNdOrdYear = "January 2nd, 2006"
	MonthDayRdOrdYear = "January 2rd, 2006"
	MonthDayStOrdYear = "January 2st, 2006"
	MonthDayThOrdYear = "January 2th, 2006"
	ShortMonthYear    = "Jan 2006"
	FullMonthYear     = "January 2006"
	// QuarterYear is actually parsed with the "Q" number as the day, this is converted in SteamDateLayout.Parse to the
	// correct date. The time.Time returned by SteamDateLayout.Parse will be a date to the first day of the quarter.
	QuarterYear = "Q2 2006"
	Year        = "2006"
)

// String returns the name of the SteamDateLayout.
func (sdf SteamDateLayout) String() string {
	switch sdf {
	case DayShortMonthYear:
		return "DayShortMonthYear"
	case DayShortMonthYearNoCommas:
		return "DayShortMonthYearNoCommas"
	case ShortMonthDayYear:
		return "ShortMonthDayYear"
	case DayShortMonthYearDots:
		return "DayShortMonthYearDots"
	case MonthDayNdOrdYear:
		return "MonthDayNdOrdYear"
	case MonthDayRdOrdYear:
		return "MonthDayRdOrdYear"
	case MonthDayStOrdYear:
		return "MonthDayStOrdYear"
	case MonthDayThOrdYear:
		return "MonthDayThOrdYear"
	case ShortMonthYear:
		return "ShortMonthYear"
	case FullMonthYear:
		return "FullMonthYear"
	case QuarterYear:
		return "QuarterYear"
	case Year:
		return "Year"
	default:
		return "<nil>"
	}
}

// Parse will attempt to parse the given date string value as a time.Time using the layout described by the
// SteamDateLayout.
//
// For some SteamDateLayout, there is extra manipulation that takes place after the date string value
// has been successfully parsed. This is evident in the QuarterYear SteamDateLayout, where the day of the parsed
// time.Time is converted to the quarter number by setting the month and day to the correct values for that quarter.
func (sdf SteamDateLayout) Parse(value string) (date time.Time, err error) {
	if date, err = time.Parse(string(sdf), value); err == nil {
		// For some SteamDateFormats we need to apply manipulations after the value has been parsed.
		switch sdf {
		// We need to convert the day in QuarterYear to the correct date for the quarter
		case QuarterYear:
			switch date.Day() {
			case 1:
				// Do nothing here as the date is already correct
			case 2:
				date = time.Date(date.Year(), 4, 1, 0, 0, 0, 0, time.UTC)
			case 3:
				date = time.Date(date.Year(), 7, 1, 0, 0, 0, 0, time.UTC)
			case 4:
				date = time.Date(date.Year(), 10, 1, 0, 0, 0, 0, time.UTC)
			default:
				err = fmt.Errorf("%s has a day of %d, expected 1, 2, 3, or 4", sdf.String(), date.Day())
			}
		default:
			break
		}
	}
	return
}

// SteamDateLayouts contains all the SteamDateLayout.
var SteamDateLayouts = []SteamDateLayout{
	DayShortMonthYear,
	DayShortMonthYearNoCommas,
	ShortMonthDayYear,
	DayShortMonthYearDots,
	MonthDayNdOrdYear,
	MonthDayRdOrdYear,
	MonthDayStOrdYear,
	MonthDayThOrdYear,
	ShortMonthYear,
	QuarterYear,
	Year,
}

// ParseSteamDate will parse the given string value to a date by attempting to parse it using each SteamDateLayout in
// SteamDateLayouts. If the date string cannot be parsed, then the error that is returned will be the merged error
// constructed from all the errors for each SteamDateLayout.
func ParseSteamDate(value string) (date time.Time, err error) {
	errs := make([]error, 0)
	for _, format := range SteamDateLayouts {
		if date, err = format.Parse(value); err != nil {
			errs = append(errs, errors.Wrapf(err, "could not parse %s using %s", value, format.String()))
		} else {
			err = nil
			break
		}
	}

	if err != nil {
		err = agem.MergeErrors(errs...)
	}
	return
}
