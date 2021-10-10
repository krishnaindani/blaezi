// Copyright 2021 Aadhav Vignesh

// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at

// 	http://www.apache.org/licenses/LICENSE-2.0

// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
)

type Inspector struct {
	client        Client
	baseURL       *url.URL
	auth          string
	workerThreads int
}

func NewInspector(client Client, baseURL *url.URL, auth string, workerThreads int) Inspector {
	return Inspector{client: client, baseURL: baseURL, auth: auth, workerThreads: workerThreads}
}

type Tests []Test

// The worker fetches tests and performs them.
func (inspector Inspector) worker(wg *sync.WaitGroup, testsChan <-chan Test, resultsChan chan<- Result, errorChan chan<- error) {
	for test := range testsChan {
		result, err := inspector.runTest(test)

		if err != nil {
			errorChan <- err
		} else {
			resultsChan <- result
		}

		wg.Done()
	}
}

// Runs tests and returns results.
func (inspector Inspector) runTest(test Test) (Result, error) {
	url := *inspector.baseURL

	// Parses URL, checks for invalid URLs
	parsedURL, err := url.Parse(path.Join(url.Path + test.URL))
	if err != nil {
		return Result{}, err
	}

	// Send a new GET request
	req, err := http.NewRequest(test.Method, parsedURL.String(), strings.NewReader(test.ReqBody))
	if err != nil {
		return Result{}, err
	}

	// Get response
	resp, err := inspector.client.Do(req)
	if err != nil {
		return Result{}, err
	}

	result := Result{StatusCode: resp.StatusCode, Test: test}

	// If content is specified in test, read response body.
	if test.Content != "" {
		defer resp.Body.Close()
		content, err := ioutil.ReadAll(resp.Body)

		if err != nil {
			return result, err
		}

		result.Content = content
	}

	return result, nil
}

// This is exposed to the main program.
func (inspector Inspector) Test(tests Tests) (results Results, errors []error) {
	// total count of tests
	numTests := len(tests)

	// Channels for handling tests, results and errors.
	testsChan := make(chan Test, numTests)
	resultsChan := make(chan Result, numTests)
	errorChan := make(chan error, numTests)

	var wg sync.WaitGroup

	// TODO: Make number of goroutines configurable
	for i := 1; i <= inspector.workerThreads; i++ {
		// send workers to process tests
		go inspector.worker(&wg, testsChan, resultsChan, errorChan)
	}

	// Send tests to the tests channel.
	for _, test := range tests {
		testsChan <- test
		wg.Add(1)
	}

	// Close test channel
	close(testsChan)

	wg.Wait()

	// fetch and append results
	for i := 0; i < numTests; i++ {
		select {
		case err := <-errorChan:
			errors = append(errors, err)
		case result := <-resultsChan:
			results = append(results, result)
		}
	}

	return results, errors
}
