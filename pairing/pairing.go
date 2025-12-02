package pairing

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/mluiten/evcc-homewizard-v2/device"
	"github.com/mluiten/evcc-homewizard-v2/discovery"
)

// PairedDevice represents a device that has been successfully paired
type PairedDevice struct {
	Host  string
	Token string
	Type  device.DeviceType
}

// DiscoverAndPairDevices executes the interactive pairing flow
func DiscoverAndPairDevices(name string, timeout int) error {
	// Validate name according to HomeWizard API requirements
	namePattern := regexp.MustCompile(`^[a-zA-Z0-9\-_/\\# ]{1,40}$`)
	if !namePattern.MatchString(name) {
		return fmt.Errorf("invalid name: must be 1-40 characters (a-z, A-Z, 0-9, -, _, \\, /, #, spaces)")
	}

	// Discovery mode
	fmt.Println("HomeWizard Device Discovery")
	fmt.Println("===========================")
	fmt.Println()
	fmt.Printf("Scanning network (max %ds)...\n", timeout)
	fmt.Println()

	devices := discoverInteractively(timeout)

	if len(devices) == 0 {
		return fmt.Errorf("no HomeWizard devices found on network 😞")
	}

	fmt.Println()
	fmt.Println("HomeWizard Device Pairing")
	fmt.Println("=========================")
	fmt.Println()
	fmt.Println("Press the button on your devices:")
	fmt.Println()

	// Pair all devices in parallel
	paired := pairDevicesParallel(devices, name)

	// Print configuration
	printHomeWizardMultiConfig(paired)

	return nil
}

// PairSingleDevice pairs a specific device without discovery
func PairSingleDevice(host, name string) error {
	// Validate name according to HomeWizard API requirements
	namePattern := regexp.MustCompile(`^[a-zA-Z0-9\-_/\\# ]{1,40}$`)
	if !namePattern.MatchString(name) {
		return fmt.Errorf("invalid name: must be 1-40 characters (a-z, A-Z, 0-9, -, _, \\, /, #, spaces)")
	}

	host = strings.TrimPrefix(host, "http://")
	host = strings.TrimPrefix(host, "https://")

	fmt.Println("HomeWizard Device Pairing")
	fmt.Println("=========================")
	fmt.Println()
	fmt.Printf("Device: %s\n", host)
	fmt.Println()
	fmt.Println("Press the button on your device:")
	fmt.Println()
	fmt.Println()

	status := deviceStatus{
		device: discovery.DiscoveredDevice{
			Host: host,
		},
		status: "initializing...",
	}

	updateStatusLine(0, &status, 1)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Pair the device
	token, err := pairDeviceWithContext(ctx, host, name, func(attempt int) {
		status.attempt = attempt
		status.status = fmt.Sprintf("waiting for button press (attempt %d/36)...", attempt)
		updateStatusLine(0, &status, 1)
	})

	if err != nil {
		fmt.Println()
		return fmt.Errorf("pairing failed: %w", err)
	}

	fmt.Println()
	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("Pairing Successful!")
	fmt.Println("========================================")
	fmt.Println()
	fmt.Printf("Token: %s\n", token)
	fmt.Println()

	return nil
}

type discoverySpinner struct {
	frames []string
	idx    int
	active bool
}

func newSpinner() *discoverySpinner {
	return &discoverySpinner{
		frames: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
		active: true,
	}
}

func (s *discoverySpinner) tick() {
	if s.active {
		s.idx = (s.idx + 1) % len(s.frames)
		fmt.Printf("\r%s Searching...", s.frames[s.idx])
	}
}

func (s *discoverySpinner) clear() {
	fmt.Print("\r\033[K")
}

func (s *discoverySpinner) stop() {
	s.clear()
	s.active = false
}

func printDiscoveredDevice(count int, device discovery.DiscoveredDevice) {
	fmt.Printf("  %d. %s (%s) at %s\n", count, device.Instance, device.Type, device.Host)
}

func confirmDevicesFound() bool {
	fmt.Println()
	fmt.Print("Is this everything? [Y/n]: ")

	var response string
	fmt.Scanln(&response)
	response = strings.ToLower(strings.TrimSpace(response))

	if response == "n" || response == "no" {
		fmt.Println()
		fmt.Println("Discovery aborted.")
		fmt.Println("Please ensure all devices are powered on and on the same network, then try again.")
		return false
	}

	return true
}

func discoverInteractively(timeoutSec int) []discovery.DiscoveredDevice {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()

	deviceChan := make(chan discovery.DiscoveredDevice, 10)
	done := make(chan struct{})

	// Start discovery
	go func() {
		discovery.DiscoverDevices(ctx, func(device discovery.DiscoveredDevice) {
			deviceChan <- device
		})
		close(done)
	}()

	devices := make([]discovery.DiscoveredDevice, 0)
	spinner := newSpinner()
	spinnerTicker := time.NewTicker(100 * time.Millisecond)
	defer spinnerTicker.Stop()

	// Stop searching after this period of no new devices
	quietPeriod := 3 * time.Second
	var quietTimer <-chan time.Time

	fmt.Printf("\r%s Searching...", spinner.frames[0])

	for {
		select {
		case device := <-deviceChan:
			// Clear spinner and print device
			spinner.clear()
			devices = append(devices, device)
			printDiscoveredDevice(len(devices), device)

			// Start/reset quiet period timer
			quietTimer = time.After(quietPeriod)

		case <-spinnerTicker.C:
			spinner.tick()

		case <-quietTimer:
			// No new devices found recently, stop searching
			cancel()
			<-done // Wait for discovery goroutine to finish
			spinner.stop()

			// Ask user if satisfied with results
			if len(devices) > 0 && !confirmDevicesFound() {
				return nil
			}
			return devices

		case <-done:
			// Overall timeout reached
			spinner.stop()

			// Ask user if satisfied with results
			if len(devices) > 0 && !confirmDevicesFound() {
				return nil
			}
			return devices
		}
	}
}

type deviceStatus struct {
	device  discovery.DiscoveredDevice
	status  string
	attempt int
	token   string
	err     error
}

func pairDevicesParallel(devices []discovery.DiscoveredDevice, name string) []PairedDevice {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	statuses := make([]*deviceStatus, len(devices))
	var statusMu sync.Mutex
	var wg sync.WaitGroup

	// Initialize and print status for each device
	for i := range devices {
		statuses[i] = &deviceStatus{
			device: devices[i],
			status: "initializing...",
		}
		fmt.Printf("[%d] %s: %s\n", i+1, statuses[i].device.Host, statuses[i].status)
	}

	totalLines := len(statuses)

	// Start pairing goroutine for each device
	for i := range devices {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			status := statuses[idx]
			device := devices[idx]

			token, err := pairDeviceWithContext(ctx, device.Host, name, func(attempt int) {
				statusMu.Lock()
				defer statusMu.Unlock()
				status.attempt = attempt
				status.status = fmt.Sprintf("waiting for button press (attempt %d/36)...", attempt)
				updateStatusLine(idx, status, totalLines)
			})

			statusMu.Lock()
			defer statusMu.Unlock()

			if err != nil {
				status.err = err
				status.status = fmt.Sprintf("✗ FAILED: %v", err)
			} else {
				status.token = token
				status.status = "✓ SUCCESS"
			}
			updateStatusLine(idx, status, totalLines)
		}(i)
	}

	wg.Wait()
	fmt.Println()

	// Build result - pre-allocate with capacity
	paired := make([]PairedDevice, 0, len(devices))
	failedCount := 0

	for _, status := range statuses {
		if status.token != "" {
			paired = append(paired, PairedDevice{
				Host:  status.device.Host,
				Token: status.token,
				Type:  status.device.Type,
			})
		} else {
			failedCount++
		}
	}

	if failedCount > 0 {
		fmt.Printf("\nWarning: %d device(s) failed to pair\n", failedCount)
	}

	return paired
}

func updateStatusLine(line int, status *deviceStatus, totalLines int) {
	// Move cursor up to the line, clear it, and print new status
	fmt.Printf("\033[%dA\r\033[K[%d] %s: %s\033[%dB\r",
		totalLines-line, line+1, status.device.Host, status.status, totalLines-line)
}

func pairDeviceWithContext(ctx context.Context, host, name string, onAttempt func(int)) (string, error) {
	uri := fmt.Sprintf("https://%s", host)

	// Create HTTP client with insecure transport for self-signed certs
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: 3 * time.Second,
	}

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	attempt := 0

	for {
		select {
		case <-ticker.C:
			attempt++
			onAttempt(attempt)

			token, err := requestToken(client, uri, name)
			if err == nil {
				return token, nil
			}

			if !isButtonPressRequired(err) {
				return "", fmt.Errorf("error: %v", err)
			}

		case <-ctx.Done():
			return "", fmt.Errorf("timeout after 3 minutes")
		}
	}
}

func requestToken(client *http.Client, uri, name string) (string, error) {
	endpoint := fmt.Sprintf("%s/api/user", uri)

	reqBody := struct {
		Name string `json:"name"`
	}{
		Name: fmt.Sprintf("local/%s", name),
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(string(jsonData)))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Version", "2")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", &httpError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	var res struct {
		Token string `json:"token"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}

	return res.Token, nil
}

type httpError struct {
	StatusCode int
	Body       string
}

func (e *httpError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Body)
}

func isButtonPressRequired(err error) bool {
	if httpErr, ok := err.(*httpError); ok {
		return httpErr.StatusCode == http.StatusForbidden
	}
	return false
}

func printHomeWizardMultiConfig(devices []PairedDevice) {
	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("Configuration Complete!")
	fmt.Println("========================================")
	fmt.Println()
	fmt.Println("Add this to your evcc.yaml configuration:")
	fmt.Println()

	// Categorize devices
	var p1Meter *PairedDevice
	kwhMeters := make([]PairedDevice, 0)
	batteries := make([]PairedDevice, 0)

	for i := range devices {
		switch devices[i].Type {
		case device.DeviceTypeP1Meter:
			if p1Meter == nil {
				p1Meter = &devices[i]
			}
		case device.DeviceTypeKWHMeter:
			kwhMeters = append(kwhMeters, devices[i])
		case device.DeviceTypeBattery:
			batteries = append(batteries, devices[i])
		default:
			// Unknown type - assume first is P1 meter if not set
			if p1Meter == nil {
				p1Meter = &devices[i]
			}
		}
	}

	fmt.Println("meters:")

	// Print P1 meter (grid) configuration
	if p1Meter != nil {
		fmt.Println("- name: grid")
		fmt.Println("  type: homewizard-v2")
		fmt.Println("  usage: grid")
		fmt.Printf("  host: %s\n", p1Meter.Host)
		fmt.Printf("  token: %s\n", p1Meter.Token)
		fmt.Println()
	}

	// Print kWh meter (pv) configurations
	for i, kwh := range kwhMeters {
		meterName := "pv"
		if i > 0 {
			meterName = fmt.Sprintf("pv%d", i+1)
		}
		fmt.Printf("- name: %s\n", meterName)
		fmt.Println("  type: homewizard-v2")
		fmt.Println("  usage: pv    # or \"charge\", if you use it for something else")
		fmt.Printf("  host: %s\n", kwh.Host)
		fmt.Printf("  token: %s\n", kwh.Token)
		fmt.Println()
	}

	// Print battery configurations
	for i, bat := range batteries {
		meterName := "battery"
		if i > 0 {
			meterName = fmt.Sprintf("battery%d", i+1)
		}
		fmt.Printf("- name: %s\n", meterName)
		fmt.Println("  type: homewizard-v2")
		fmt.Println("  usage: battery")
		fmt.Printf("  host: %s\n", bat.Host)
		fmt.Printf("  token: %s\n", bat.Token)

		fmt.Println()
	}

	// Print helpful notes
	if len(devices) > 0 {
		fmt.Println("# Notes:")
		fmt.Println("# - Make sure to add these devices to your \"sites\" as well")
		fmt.Println()
	}
}
