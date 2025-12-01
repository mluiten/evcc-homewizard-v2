package device

import (
	"time"

	"github.com/evcc-io/evcc/util"
)

// KWHMeterDevice represents a kWh meter (HWE-KWH1, HWE-KWH3)
type KWHMeterDevice struct {
	*baseMeterDevice[KWHMeasurement]
}

// NewKWHMeterDevice creates a new kWh meter device instance
func NewKWHMeterDevice(host, token string, timeout time.Duration) *KWHMeterDevice {
	d := &KWHMeterDevice{
		baseMeterDevice: &baseMeterDevice[KWHMeasurement]{
			deviceBase:  newDeviceBase(DeviceTypeKWHMeter, host, token, timeout),
			measurement: util.NewMonitor[KWHMeasurement](timeout),
		},
	}

	// Create connection with message handler
	d.conn = NewConnection(host, token, d.handleMessage)

	return d
}

// GetTotalEnergy returns the total energy for kWh meters (simple totals)
// For PV usage: returns export energy (production)
// For grid usage: returns import energy (consumption)
func (d *KWHMeterDevice) GetTotalEnergy(usePVExport bool) (float64, error) {
	m, err := d.GetMeasurement()
	if err != nil {
		return 0, err
	}

	if usePVExport {
		return m.EnergyExportkWh, nil
	}
	return m.EnergyImportkWh, nil
}
