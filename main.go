package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"runtime/pprof"
	"strings"
	"sync"
	"time"
)

/*
	Design Plan

whiteboard: https://miro.com/app/board/uXjVGL2ZYSQ=/?share_link_id=504796011623
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



type isUpRes struct {
	isUp       bool
	statusCode int
}

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

func isUp(url string) isUpRes {
	result := isUpRes{}
	client := http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Timeout: 10 * time.Second,
	}
	req, err := http.NewRequest("Get", "https://"+url, nil)
	agent := "wcheck/1.0 (Language=Go)"
	req.Header.Set("User-Agent", agent)
	res, err := client.Do(req)
	if err != nil {
		fmt.Println(err)
		result.isUp = false
		result.statusCode = -1
		return result
	}
	res.Body.Close()
	result.isUp = res.StatusCode >= 200 && res.StatusCode <= 420
	result.statusCode = res.StatusCode
	return result
}

func isValidUrl(url string) bool {
	re := regexp.MustCompile(`^(https?://)?([\da-z.-]+)\.([a-z.]{2,6})([/\w .-]*)*/?$`)
	return re.MatchString(url)
}

func giveUrls(filename string) []string {
	fileByte, err := os.ReadFile(filename)
	if err != nil {
		panic(err)
	}
	urls := strings.Split(string(fileByte), "\n")
	FinalUrls := []string{}
	for _, val := range urls {
		if isValidUrl(val) {
			FinalUrls = append(FinalUrls, val)
		}
	}
	return FinalUrls
}

func main() {
	fmt.Println("Welcome to wcheck !!")

	//arguments handling
	var numOfWorkers int
	flag.IntVar(&numOfWorkers, "n", 10, "define number of Workers needed, keep it below 10k or else bye bye system")
	var filename string
	flag.StringVar(&filename, "w", "./majestic", "choose the wordlist of urls needed")
	flag.Parse()

	Finalurls := giveUrls(filename)

	// track for Ctrl + C Interrupt
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

	//make channels
	jobs := make(chan string, len(Finalurls))
	Result := make(chan bool, len(Finalurls))

	//make workers
	var wg sync.WaitGroup
	for i := 0; i < numOfWorkers; i++ {
		wg.Go(func() {
			Worker(i, jobs, Result)
		})
	}

	//fill channels
	for _, url := range Finalurls {
		jobs <- url
	}
	//increment counters
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
