package device

import (
	"encoding/json"
	"fmt"

	"github.com/evcc-io/evcc/api"
	"github.com/evcc-io/evcc/util"
)

// MeterMeasurement contains power and energy data from any HomeWizard meter (P1 or kWh)
// This unified structure supports both 1-phase and 3-phase configurations
type MeterMeasurement struct {
	// Power measurements
	PowerW   float64 `json:"power_w"`
	PowerL1W float64 `json:"power_l1_w"` // 3-phase only
	PowerL2W float64 `json:"power_l2_w"` // 3-phase only
	PowerL3W float64 `json:"power_l3_w"` // 3-phase only

	// Voltage measurements
	VoltageV   float64 `json:"voltage_v"`    // 1-phase or aggregate (P1)
	VoltageL1V float64 `json:"voltage_l1_v"` // 3-phase
	VoltageL2V float64 `json:"voltage_l2_v"` // 3-phase
	VoltageL3V float64 `json:"voltage_l3_v"` // 3-phase

	// Current measurements
	CurrentA   float64 `json:"current_a"`
	CurrentL1A float64 `json:"current_l1_a"` // 3-phase
	CurrentL2A float64 `json:"current_l2_a"` // 3-phase
	CurrentL3A float64 `json:"current_l3_a"` // 3-phase

	// Energy measurements - simple totals (kWh meters)
	EnergyImportkWh float64 `json:"energy_import_kwh"`
	EnergyExportkWh float64 `json:"energy_export_kwh"`

	// Energy measurements - tariff breakdown (P1 meters)
	EnergyImportT1kWh float64 `json:"energy_import_t1_kwh"`
	EnergyImportT2kWh float64 `json:"energy_import_t2_kwh"`
	EnergyExportT1kWh float64 `json:"energy_export_t1_kwh"`
	EnergyExportT2kWh float64 `json:"energy_export_t2_kwh"`
}

// baseMeterDevice provides common functionality for P1 and kWh meters
type baseMeterDevice struct {
	*deviceBase
	measurement *util.Monitor[MeterMeasurement]
}

// GetMeasurement returns the latest meter measurement data
func (d *baseMeterDevice) GetMeasurement() (MeterMeasurement, error) {
	m, err := d.measurement.Get()
	if err != nil {
		return MeterMeasurement{}, api.ErrTimeout
	}
	return m, nil
}

// handleMessage routes incoming WebSocket messages for meter
func (d *baseMeterDevice) handleMessage(msgType string, data json.RawMessage) error {
	switch msgType {
	case "measurement":
		var m MeterMeasurement
		if err := json.Unmarshal(data, &m); err != nil {
			return fmt.Errorf("unmarshal meter measurement: %w", err)
		}
		d.measurement.Set(m)
		d.log.TRACE.Printf("updated meter measurement: power=%.1fW", m.PowerW)

	case "device", "system":
		// Ignore device info and system messages
		d.log.TRACE.Printf("ignoring message type: %s", msgType)

	default:
		d.log.TRACE.Printf("unknown message type: %s", msgType)
	}

	return nil
}

// Common methods for all meter devices

// GetPower returns the total power, optionally inverted for PV usage
func (d *baseMeterDevice) GetPower(invertForPV bool) (float64, error) {
	m, err := d.GetMeasurement()
	if err != nil {
		return 0, err
	}

	power := m.PowerW
	if invertForPV {
		power = -power
	}

	return power, nil
}

// GetPhasePowers returns the per-phase powers for 1-phase or 3-phase meters
// For 1-phase: returns (totalPower, 0, 0)
// For 3-phase: returns (L1, L2, L3)
func (d *baseMeterDevice) GetPhasePowers(phases int, invertForPV bool) (float64, float64, float64, error) {
	m, err := d.GetMeasurement()
	if err != nil {
		return 0, 0, 0, err
	}

	var p1, p2, p3 float64

	if phases == 1 {
		p1 = m.PowerW
	} else {
		p1, p2, p3 = m.PowerL1W, m.PowerL2W, m.PowerL3W
	}

	if invertForPV {
		p1, p2, p3 = -p1, -p2, -p3
	}

	return p1, p2, p3, nil
}

// GetPhaseCurrents returns the per-phase currents for 1-phase or 3-phase meters
// For 1-phase: returns (totalCurrent, 0, 0)
// For 3-phase: returns (L1, L2, L3)
func (d *baseMeterDevice) GetPhaseCurrents(phases int) (float64, float64, float64, error) {
	m, err := d.GetMeasurement()
	if err != nil {
		return 0, 0, 0, err
	}

	if phases == 1 {
		return m.CurrentA, 0, 0, nil
	}

	return m.CurrentL1A, m.CurrentL2A, m.CurrentL3A, nil
}

// GetPhaseVoltages returns the per-phase voltages for 1-phase or 3-phase meters
// For 1-phase: returns (voltage, 0, 0)
// For 3-phase: returns (L1, L2, L3)
func (d *baseMeterDevice) GetPhaseVoltages(phases int) (float64, float64, float64, error) {
	m, err := d.GetMeasurement()
	if err != nil {
		return 0, 0, 0, err
	}

	if phases == 1 {
		return m.VoltageV, 0, 0, nil
	}

	return m.VoltageL1V, m.VoltageL2V, m.VoltageL3V, nil
}
