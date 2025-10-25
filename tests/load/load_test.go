package load

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"auth/internal/models"
)

// LoadTestConfig defines configuration for load testing
type LoadTestConfig struct {
	BaseURL         string
	Concurrency     int
	RequestsPerUser int
	Duration        time.Duration
	RampUpTime      time.Duration
}

// TestResult holds the results of a load test
type TestResult struct {
	TotalRequests     int64
	SuccessfulReqs    int64
	FailedReqs        int64
	AvgResponseTime   time.Duration
	MinResponseTime   time.Duration
	MaxResponseTime   time.Duration
	P95ResponseTime   time.Duration
	P99ResponseTime   time.Duration
	RequestsPerSecond float64
	Errors            map[string]int64
}

// LoadTester manages load testing execution
type LoadTester struct {
	config *LoadTestConfig
	client *http.Client
}

// NewLoadTester creates a new load tester
func NewLoadTester(config *LoadTestConfig) *LoadTester {
	return &LoadTester{
		config: config,
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// RunSignUpLoadTest runs load test for signup endpoint
func (lt *LoadTester) RunSignUpLoadTest(ctx context.Context) (*TestResult, error) {
	return lt.runLoadTest(ctx, "signup", func(userID int) (*http.Request, error) {
		signupData := models.SignUpRequest{
			Username: fmt.Sprintf("loadtest_user_%d_%d", time.Now().Unix(), userID),
			Email:    fmt.Sprintf("loadtest_%d_%d@example.com", time.Now().Unix(), userID),
			Password: "LoadTest123!",
		}

		jsonData, err := json.Marshal(signupData)
		if err != nil {
			return nil, err
		}

		req, err := http.NewRequest("POST", lt.config.BaseURL+"/signup", bytes.NewBuffer(jsonData))
		if err != nil {
			return nil, err
		}

		req.Header.Set("Content-Type", "application/json")
		return req, nil
	})
}

// RunLoginLoadTest runs load test for login endpoint
func (lt *LoadTester) RunLoginLoadTest(ctx context.Context, username, password string) (*TestResult, error) {
	return lt.runLoadTest(ctx, "login", func(userID int) (*http.Request, error) {
		loginData := models.LoginRequest{
			Username: username,
			Password: password,
		}

		jsonData, err := json.Marshal(loginData)
		if err != nil {
			return nil, err
		}

		req, err := http.NewRequest("POST", lt.config.BaseURL+"/login", bytes.NewBuffer(jsonData))
		if err != nil {
			return nil, err
		}

		req.Header.Set("Content-Type", "application/json")
		return req, nil
	})
}

// RunProfileLoadTest runs load test for profile endpoint
func (lt *LoadTester) RunProfileLoadTest(ctx context.Context, token string) (*TestResult, error) {
	return lt.runLoadTest(ctx, "profile", func(userID int) (*http.Request, error) {
		req, err := http.NewRequest("GET", lt.config.BaseURL+"/profile", nil)
		if err != nil {
			return nil, err
		}

		req.Header.Set("Authorization", "Bearer "+token)
		return req, nil
	})
}

func (lt *LoadTester) runLoadTest(ctx context.Context, testName string, reqBuilder func(int) (*http.Request, error)) (*TestResult, error) {
	result := &TestResult{
		Errors:          make(map[string]int64),
		MinResponseTime: time.Hour, // Initialize with high value
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	responseTimes := make([]time.Duration, 0)
	
	startTime := time.Now()
	
	// Channel to control request rate during ramp-up
	requestChan := make(chan struct{}, lt.config.Concurrency)
	
	// Ramp-up phase
	rampUpInterval := lt.config.RampUpTime / time.Duration(lt.config.Concurrency)
	
	for i := 0; i < lt.config.Concurrency; i++ {
		go func(workerID int) {
			// Wait for ramp-up
			time.Sleep(time.Duration(workerID) * rampUpInterval)
			
			for j := 0; j < lt.config.RequestsPerUser; j++ {
				select {
				case <-ctx.Done():
					return
				default:
					wg.Add(1)
					requestChan <- struct{}{}
					
					go func(reqID int) {
						defer wg.Done()
						defer func() { <-requestChan }()
						
						req, err := reqBuilder(reqID)
						if err != nil {
							mu.Lock()
							result.FailedReqs++
							result.Errors["request_build_error"]++
							mu.Unlock()
							return
						}
						
						reqStart := time.Now()
						resp, err := lt.client.Do(req)
						reqDuration := time.Since(reqStart)
						
						mu.Lock()
						result.TotalRequests++
						responseTimes = append(responseTimes, reqDuration)
						
						if reqDuration < result.MinResponseTime {
							result.MinResponseTime = reqDuration
						}
						if reqDuration > result.MaxResponseTime {
							result.MaxResponseTime = reqDuration
						}
						
						if err != nil {
							result.FailedReqs++
							result.Errors["network_error"]++
						} else {
							defer resp.Body.Close()
							if resp.StatusCode >= 200 && resp.StatusCode < 300 {
								result.SuccessfulReqs++
							} else {
								result.FailedReqs++
								result.Errors[fmt.Sprintf("http_%d", resp.StatusCode)]++
							}
						}
						mu.Unlock()
					}(j)
				}
			}
		}(i)
	}
	
	wg.Wait()
	
	// Calculate statistics
	totalDuration := time.Since(startTime)
	result.RequestsPerSecond = float64(result.TotalRequests) / totalDuration.Seconds()
	
	if len(responseTimes) > 0 {
		// Calculate average
		var totalTime time.Duration
		for _, t := range responseTimes {
			totalTime += t
		}
		result.AvgResponseTime = totalTime / time.Duration(len(responseTimes))
		
		// Calculate percentiles
		result.P95ResponseTime = calculatePercentile(responseTimes, 95)
		result.P99ResponseTime = calculatePercentile(responseTimes, 99)
	}
	
	return result, nil
}

// calculatePercentile calculates the given percentile from response times
func calculatePercentile(times []time.Duration, percentile float64) time.Duration {
	if len(times) == 0 {
		return 0
	}
	
	// Sort the times (simple bubble sort for small datasets)
	sorted := make([]time.Duration, len(times))
	copy(sorted, times)
	
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	
	index := int(percentile/100*float64(len(sorted)-1) + 0.5)
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	
	return sorted[index]
}

// PrintResults prints the load test results in a formatted way
func (result *TestResult) PrintResults(testName string) {
	fmt.Printf("\n=== Load Test Results: %s ===\n", testName)
	fmt.Printf("Total Requests: %d\n", result.TotalRequests)
	fmt.Printf("Successful: %d (%.2f%%)\n", result.SuccessfulReqs, 
		float64(result.SuccessfulReqs)/float64(result.TotalRequests)*100)
	fmt.Printf("Failed: %d (%.2f%%)\n", result.FailedReqs,
		float64(result.FailedReqs)/float64(result.TotalRequests)*100)
	fmt.Printf("Requests/sec: %.2f\n", result.RequestsPerSecond)
	fmt.Printf("Avg Response Time: %v\n", result.AvgResponseTime)
	fmt.Printf("Min Response Time: %v\n", result.MinResponseTime)
	fmt.Printf("Max Response Time: %v\n", result.MaxResponseTime)
	fmt.Printf("95th Percentile: %v\n", result.P95ResponseTime)
	fmt.Printf("99th Percentile: %v\n", result.P99ResponseTime)
	
	if len(result.Errors) > 0 {
		fmt.Printf("\nErrors:\n")
		for errorType, count := range result.Errors {
			fmt.Printf("  %s: %d\n", errorType, count)
		}
	}
	fmt.Printf("=====================================\n\n")
}