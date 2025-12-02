package device

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/evcc-io/evcc/api"
	"github.com/evcc-io/evcc/util"
	"github.com/evcc-io/evcc/util/request"
)

// P1MeterDevice represents a P1 meter (HWE-P1) with battery control
type P1MeterDevice struct {
	*baseMeterDevice[P1Measurement]
	batteriesData *util.Monitor[BatteriesData]
}

// NewP1MeterDevice creates a new P1 meter device instance
func NewP1MeterDevice(host, token string, timeout time.Duration) *P1MeterDevice {
	d := &P1MeterDevice{
		baseMeterDevice: &baseMeterDevice[P1Measurement]{
			deviceBase:  newDeviceBase(DeviceTypeP1Meter, host, token, timeout),
			measurement: util.NewMonitor[P1Measurement](timeout),
		},
		batteriesData: util.NewMonitor[BatteriesData](timeout),
	}

	// Create connection with message handler, subscribe to measurement and batteries topics
	d.conn = NewConnection(host, token, d.handleP1Message, "measurement", "batteries")

	return d
}

// handleP1Message extends base message handling with battery messages
func (d *P1MeterDevice) handleP1Message(msgType string, data json.RawMessage) error {
	switch msgType {
	case "batteries":
		var b BatteriesData
		if err := json.Unmarshal(data, &b); err != nil {
			return fmt.Errorf("unmarshal batteries data: %w", err)
		}
		d.batteriesData.Set(b)
		return nil

	default:
		// Delegate to baseMeterDevice for measurement and other messages
		return d.baseMeterDevice.handleMessage(msgType, data)
	}
}

// GetPower returns the total power for P1 meters (always grid, never inverted)
func (d *P1MeterDevice) GetPower() (float64, error) {
	m, err := d.GetMeasurement()
	if err != nil {
		return 0, err
	}
	return m.GetCommon().PowerW, nil
}

// GetPhasePowers returns the per-phase powers for P1 meters (always grid, never inverted)
// For 1-phase: returns (totalPower, 0, 0)
// For 3-phase: returns (L1, L2, L3)
func (d *P1MeterDevice) GetPhasePowers(phases int) (float64, float64, float64, error) {
	m, err := d.GetMeasurement()
	if err != nil {
		return 0, 0, 0, err
	}

	common := m.GetCommon()
	if phases == 1 {
		return common.PowerW, 0, 0, nil
	}
	return common.PowerL1W, common.PowerL2W, common.PowerL3W, nil
}

// GetTotalEnergy returns the total import energy for P1 meters (sum of T1 and T2 tariffs)
// P1 meters are always grid meters, so we always return import energy
func (d *P1MeterDevice) GetTotalEnergy() (float64, error) {
	m, err := d.GetMeasurement()
	if err != nil {
		return 0, err
	}
	return m.EnergyImportT1kWh + m.EnergyImportT2kWh, nil
}

// GetBatteryPowerLimits returns the battery power limits (charge, discharge in W)
func (d *P1MeterDevice) GetBatteryPowerLimits() (float64, float64, error) {
	b, err := d.batteriesData.Get()
	if err != nil {
		return 0, 0, api.ErrTimeout
	}
	return b.MaxConsumptionW, b.MaxProductionW, nil
}

// SetBatteryMode sets the battery control mode via P1 meter
func (d *P1MeterDevice) SetBatteryMode(mode string) error {
	d.log.INFO.Printf("setting battery mode to: %s", mode)

	// Try WebSocket control first
	wsMsg := map[string]any{
		"type": "batteries",
		"data": map[string]string{"mode": mode},
	}

	if err := d.conn.Send(wsMsg); err != nil {
		d.log.DEBUG.Printf("WebSocket battery control failed, falling back to HTTP: %v", err)
		return d.setBatteryModeHTTP(mode)
	}

	// Give the device a moment to process
	time.Sleep(100 * time.Millisecond)
	d.log.DEBUG.Println("WebSocket battery control sent")
	return nil
}

// setBatteryModeHTTP sets battery mode via HTTP PUT
func (d *P1MeterDevice) setBatteryModeHTTP(mode string) error {
	uri := fmt.Sprintf("https://%s/api/batteries", d.host)
	d.log.INFO.Printf("sending HTTP PUT to %s with mode: %s", uri, mode)

	reqBody := struct {
		Mode string `json:"mode"`
	}{
		Mode: mode,
	}

	req, err := request.New(http.MethodPut, uri, request.MarshalJSON(reqBody), request.JSONEncoding)
	if err != nil {
		d.log.ERROR.Printf("failed to create HTTP request: %v", err)
		return err
	}

	// Set required headers for HomeWizard API v2
	req.Header.Set("Authorization", "Bearer "+d.token)
	req.Header.Set("X-Api-Version", "2")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	var res BatteriesData
	if err := d.DoJSON(req, &res); err != nil {
		d.log.ERROR.Printf("HTTP request failed: %v", err)
		return err
	}

	d.log.INFO.Printf("battery mode set successfully via HTTP: %s (response: mode=%s, power=%.1fW)", mode, res.Mode, res.PowerW)
	return nil
}
