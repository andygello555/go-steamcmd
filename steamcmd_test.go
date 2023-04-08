package steamcmd

import (
	"bufio"
	"fmt"
	"github.com/andygello555/url-fmt"
	"math/rand"
	"os"
	"sync"
	"testing"
	"time"
)

func ExampleSteamCMD_Flow() {
	var err error
	cmd := New(true)

	if err = cmd.Flow(
		NewCommandWithArgs(AppInfoPrint, 477160),
		NewCommandWithArgs(Quit),
	); err != nil {
		fmt.Printf("Could not execute flow: %s\n", err.Error())
	}
	fmt.Println(cmd.ParsedOutputs[0].(map[string]any)["common"].(map[string]any)["name"])
	// Output:
	// Human: Fall Flat
}

type steamCMDFlowJob struct {
	appID int
	jobID int
}

type steamCMDFlowResult struct {
	jobID        int
	appID        int
	parsedOutput any
	err          error
}

func steamCMDFlowWorker(wg *sync.WaitGroup, jobs <-chan *steamCMDFlowJob, results chan<- *steamCMDFlowResult) {
	defer wg.Done()
	for job := range jobs {
		var err error
		cmd := New(true)
		err = cmd.Flow(
			NewCommandWithArgs(AppInfoPrint, job.appID),
			NewCommandWithArgs(Quit),
		)
		results <- &steamCMDFlowResult{
			jobID:        job.jobID,
			appID:        job.appID,
			parsedOutput: cmd.ParsedOutputs[0],
			err:          err,
		}
	}
}

const (
	sampleGameWebsitesPath            = "samples/sampleGameWebsites.txt"
	steamAppPage           urlfmt.URL = "%s://store.steampowered.com/app/%d"
)

func benchmarkSteamCMDFlow(workers int, b *testing.B) {
	var err error
	s := rand.NewSource(time.Now().UTC().Unix())
	r := rand.New(s)

	// First we load the appIDs from the sample game websites into an array
	sampleAppIDs := make([]int, 0)
	var file *os.File
	if file, err = os.Open(sampleGameWebsitesPath); err != nil {
		b.Fatalf("Cannot open %s: %s", sampleGameWebsitesPath, err.Error())
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		// Parse the scanned text to a URL
		text := scanner.Text()
		if steamAppPage.Match(text) {
			args := steamAppPage.ExtractArgs(text)
			appID := args[0].(int64)
			sampleAppIDs = append(sampleAppIDs, int(appID))
		}
	}

	if err = file.Close(); err != nil {
		b.Fatalf("Could not open %s: %s", sampleGameWebsitesPath, err.Error())
	}

	// Then we start our workers
	jobs := make(chan *steamCMDFlowJob, b.N)
	results := make(chan *steamCMDFlowResult, b.N)
	var wg sync.WaitGroup

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go steamCMDFlowWorker(&wg, jobs, results)
	}

	// We reset the timer as we have completed the setup of the benchmark then queue up all our jobs. We use a random
	// appID from the sampleAppIDs array.
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		jobs <- &steamCMDFlowJob{
			appID: sampleAppIDs[r.Intn(len(sampleAppIDs))],
			jobID: i,
		}
	}

	// Close the channels and wait for the workers to finish.
	close(jobs)
	wg.Wait()
	close(results)

	// Finally, we read each result from the closed channel to see if we have any errors or parsed outputs that cannot
	// be asserted to a map.
	for result := range results {
		if _, ok := result.parsedOutput.(map[string]any); result.err != nil || !ok {
			b.Errorf(
				"Error occurred (%v)/parsed output could not be asserted to map (output: %v), in job no. %d (appID: %d)",
				result.err, result.parsedOutput, result.jobID, result.appID,
			)
		}
	}
}

func BenchmarkSteamCMD_Flow5(b *testing.B)  { benchmarkSteamCMDFlow(5, b) }
func BenchmarkSteamCMD_Flow10(b *testing.B) { benchmarkSteamCMDFlow(10, b) }

func ExampleParseSteamDate() {
	fmt.Println(ParseSteamDate("8 Oct, 2019"))
	fmt.Println(ParseSteamDate("8 Oct 2019"))
	fmt.Println(ParseSteamDate("Oct 8, 2019"))
	fmt.Println(ParseSteamDate("8. Oct. 2019"))
	fmt.Println(ParseSteamDate("July 2nd, 2021"))
	fmt.Println(ParseSteamDate("July 1st, 2021"))
	fmt.Println(ParseSteamDate("July 3rd, 2021"))
	fmt.Println(ParseSteamDate("July 30th, 2021"))
	fmt.Println(ParseSteamDate("Sep 2021"))
	fmt.Println(ParseSteamDate("Q1 2021"))
	fmt.Println(ParseSteamDate("Q2 2021"))
	fmt.Println(ParseSteamDate("Q3 2021"))
	fmt.Println(ParseSteamDate("Q4 2021"))
	fmt.Println(ParseSteamDate("2022"))
	fmt.Println(ParseSteamDate("Coming Soon"))
	// Output:
	// 2019-10-08 00:00:00 +0000 UTC <nil>
	// 2019-10-08 00:00:00 +0000 UTC <nil>
	// 2019-10-08 00:00:00 +0000 UTC <nil>
	// 2019-10-08 00:00:00 +0000 UTC <nil>
	// 2021-07-02 00:00:00 +0000 UTC <nil>
	// 2021-07-01 00:00:00 +0000 UTC <nil>
	// 2021-07-03 00:00:00 +0000 UTC <nil>
	// 2021-07-30 00:00:00 +0000 UTC <nil>
	// 2021-09-01 00:00:00 +0000 UTC <nil>
	// 2021-01-01 00:00:00 +0000 UTC <nil>
	// 2021-04-01 00:00:00 +0000 UTC <nil>
	// 2021-07-01 00:00:00 +0000 UTC <nil>
	// 2021-10-01 00:00:00 +0000 UTC <nil>
	// 2022-01-01 00:00:00 +0000 UTC <nil>
	// 0001-01-01 00:00:00 +0000 UTC could not parse Coming Soon using DayShortMonthYear: parsing time "Coming Soon" as "2 Jan, 2006": cannot parse "Coming Soon" as "2"; could not parse Coming Soon using DayShortMonthYearNoCommas: parsing time "Coming Soon" as "2 Jan 2006": cannot parse "Coming Soon" as "2"; could not parse Coming Soon using ShortMonthDayYear: parsing time "Coming Soon" as "Jan 2, 2006": cannot parse "Coming Soon" as "Jan"; could not parse Coming Soon using DayShortMonthYearDots: parsing time "Coming Soon" as "2. Jan. 2006": cannot parse "Coming Soon" as "2"; could not parse Coming Soon using MonthDayNdOrdYear: parsing time "Coming Soon" as "January 2nd, 2006": cannot parse "Coming Soon" as "January"; could not parse Coming Soon using MonthDayRdOrdYear: parsing time "Coming Soon" as "January 2rd, 2006": cannot parse "Coming Soon" as "January"; could not parse Coming Soon using MonthDayStOrdYear: parsing time "Coming Soon" as "January 2st, 2006": cannot parse "Coming Soon" as "January"; could not parse Coming Soon using MonthDayThOrdYear: parsing time "Coming Soon" as "January 2th, 2006": cannot parse "Coming Soon" as "January"; could not parse Coming Soon using ShortMonthYear: parsing time "Coming Soon" as "Jan 2006": cannot parse "Coming Soon" as "Jan"; could not parse Coming Soon using QuarterYear: parsing time "Coming Soon" as "Q2 2006": cannot parse "Coming Soon" as "Q"; could not parse Coming Soon using Year: parsing time "Coming Soon" as "2006": cannot parse "Coming Soon" as "2006"
}
