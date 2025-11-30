package device

import (
	"fmt"
	"time"

	"github.com/evcc-io/evcc/util"
	"github.com/evcc-io/evcc/util/request"
	"github.com/evcc-io/evcc/util/transport"
)

// DeviceType identifies the type of HomeWizard device
type DeviceType string

const (
	DeviceTypeP1Meter  DeviceType = "p1meter"
	DeviceTypeKWHMeter DeviceType = "kwhmeter"
	DeviceTypeBattery  DeviceType = "battery"
)

// Default configuration values
const (
	DefaultTimeout         = 30 * time.Second
	DefaultMaxCharge       = 800.0 // W - Default charge limit for HWE-BAT
	DefaultMaxDischarge    = 800.0 // W - Default discharge limit for HWE-BAT
	DefaultBatteryCapacity = 2.47  // kWh - HWE-BAT capacity
)

// deviceBase contains common functionality for all HomeWizard devices
type deviceBase struct {
	*request.Helper
	deviceType DeviceType
	host       string
	token      string
	log        *util.Logger
	conn       *Connection
	timeout    time.Duration
}

// newDeviceBase creates a new base device with common fields
func newDeviceBase(deviceType DeviceType, host, token string, timeout time.Duration) *deviceBase {
	log := util.NewLogger("homewizard-v2").Redact(token)

	d := &deviceBase{
		Helper:     request.NewHelper(log),
		deviceType: deviceType,
		host:       host,
		token:      token,
		log:        log,
		timeout:    timeout,
	}

	// Use insecure HTTPS transport for self-signed certificates
	d.Client.Transport = transport.Insecure()

	// Set timeout for HTTP requests
	d.Client.Timeout = 10 * time.Second

	return d
}

// Type returns the device type
func (d *deviceBase) Type() DeviceType {
	return d.deviceType
}

// Host returns the device hostname/IP
func (d *deviceBase) Host() string {
	return d.host
}

// Start initiates the WebSocket connection
func (d *deviceBase) Start(errC chan error) {
	d.conn.Start(errC)
}

// StartAndWait initiates the WebSocket connection and waits for it to succeed or timeout
func (d *deviceBase) StartAndWait(timeout time.Duration) error {
	errC := make(chan error, 1)
	d.Start(errC)

	select {
	case err := <-errC:
		if err != nil {
			d.Stop()
			return fmt.Errorf("connecting to device: %w", err)
		}
		return nil
	case <-time.After(timeout):
		d.Stop()
		return fmt.Errorf("connection timeout")
	}
}

// Stop gracefully closes the connection
func (d *deviceBase) Stop() {
	d.conn.Stop()
}
