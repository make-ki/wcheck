package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"runtime/pprof"
	"sync"
	"time"
)

/*
	Design Plan

whiteboard: https://miro.com/app/board/uXjVGL2ZYSQ=/?share_link_id=504796011623
Theres probably a deadlock due to the way I am using channels, will learn about them deeply
before implementing a fix. We sure are getting lucky here with our outputs working atleast.
*
* create a slice of all url (additionally create a text file or fetch it from somewhere)
1. create a slice of url
2. added url validator let gemini handle regex i aint writing that shit
3. filter urls through the isValidUrl and get a FinalUrls slice
*
1. add a waitgroup so that main() exits only after routines have returned something to channels
2. add workers so that this is not a DDoS generator for big files
3. put in stuff from FinalUrls into jobs channel, close the jobs channel
4. statusCode should be printed so made a result struct for isUp
*
if website is up print [UP] url else [DOWN] url
*/

// Workers write to stdout if a url in jobs channel is up or down
// They also write true or false for the same in Result channel
func Worker(_ int, jobs <-chan string, Result chan<- bool) {
	for url := range jobs {
		ok := isUp(url)
		if ok.isUp {
			fmt.Printf("[UP %d] %s \n", ok.statusCode, url)
			Result <- true
		} else {
			fmt.Printf("[DOWN %d] %s \n", ok.statusCode, url)
			Result <- false
		}
	}
}

// isUpResult bundles the output of isUp function
type isUpResult struct {
	isUp       bool
	statusCode int
}

// isUP sends a GET request to provided URL
// implicit http.CheckRedirect calls have been turned off by the http client
// Returns struct isUpResult based on response's statusCode
func isUp(url string) isUpResult {
	result := isUpResult{statusCode: -1}
	client := http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequest(http.MethodGet, "https://"+url, nil)
	if err != nil {
		fmt.Printf("%s\n", err)
		return result
	}

	req.Header.Set("User-Agent", "wcheck/1.0 (Language=Go)")

	res, err := client.Do(req)
	if err != nil {
		fmt.Printf("%s\n", err)
		return result
	}
	res.Body.Close()

	result.isUp = res.StatusCode >= 200 && res.StatusCode <= 420
	result.statusCode = res.StatusCode
	return result
}

// isValidURL uses regex to return true for valid urls
func isValidURL(url string) bool {
	re := regexp.MustCompile(`^(https?://)?([\da-z.-]+)\.([a-z.]{2,6})([/\w .-]*)*/?$`)
	return re.MatchString(url)
}

// parseURLS returns a slice of filtered valid urls
func parseURLS(r io.Reader) []string {
	var validURL []string
	ns := bufio.NewScanner(r)
	for ns.Scan() {
		text := ns.Text()
		if isValidURL(text) {
			validURL = append(validURL, text)
		}
	}
	return validURL
}

func main() {
	fmt.Println("Welcome to wcheck !!")

	// Get inputFile and number of workers system can afford
	var numOfWorkers int
	var inputFile string
	flag.IntVar(&numOfWorkers, "n", 10, "define number of Workers needed, keep it below 10k.")
	flag.StringVar(&inputFile, "w", "./majestic", "choose the wordlist of urls needed")
	flag.Parse()

	fd, err := os.Open(inputFile)
	if err != nil {
		fmt.Printf("%s\n", err)
		return
	}

	URLS := parseURLS(fd)

	// Track for Ctrl + C Interrupt
	successCount := 0
	failCount := 0
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for sig := range c {
			fmt.Printf("UP: %d, DOWN: %d stopped %v", successCount, failCount, sig)
			pprof.StopCPUProfile()
			os.Exit(1)
		}
	}()

	// Make workers
	jobs := make(chan string, len(URLS))
	Result := make(chan bool, len(URLS))

	var wg sync.WaitGroup
	for i := 0; i < numOfWorkers; i++ {
		wg.Go(func() {
			Worker(i, jobs, Result)
		})
	}

	// Keep signalling in the jobs channel
	for _, url := range URLS {
		jobs <- url
	}

	// Count success and failures from Result channel
	for res := range Result {
		if res {
			successCount++
		} else {
			failCount++
		}
	}
	close(jobs)
	close(Result)
	wg.Wait()
}
