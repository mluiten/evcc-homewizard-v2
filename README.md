# HomeWizard Energy API v2 - Go Client Library

Go client library for HomeWizard Energy devices using the WebSocket API v2.

## Features

- **Real-time WebSocket communication** with HomeWizard Energy devices
- **Device discovery** via mDNS/Zeroconf
- **Three device types supported**:
  - **P1 Meter** (HWE-P1): Grid monitoring with battery control
  - **kWh Meter** (HWE-KWH1/3): PV/consumption monitoring
  - **Battery** (HWE-BAT): Battery monitoring (SoC, power, cycles)
- **Automatic reconnection** with configurable retry delay
- **Thread-safe** operations with proper synchronization

## Installation

```bash
go get github.com/mluiten/evcc-homewizard-v2
```

## Quick Start

### Discover Devices

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/mluiten/evcc-homewizard-v2/discovery"
)

func main() {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    err := discovery.DiscoverDevices(ctx, func(device discovery.DiscoveredDevice) {
        fmt.Printf("Found: %s (%s) at %s:%d\n",
            device.Instance, device.Type, device.Host, device.Port)
    })

    if err != nil {
        panic(err)
    }
}
```

### P1 Meter (Grid + Battery Control)

```go
package main

import (
    "fmt"
    "time"

    "github.com/mluiten/evcc-homewizard-v2/device"
)

func main() {
    // Create P1 device
    p1 := device.NewP1Device("192.168.1.10", "your-token-here", 30*time.Second)
    defer p1.Stop()

    // Start connection and wait (new helper method)
    if err := p1.StartAndWait(5 * time.Second); err != nil {
        panic(err)
    }

    // Get measurements
    meas, err := p1.GetMeasurement()
    if err != nil {
        panic(err)
    }
    fmt.Printf("Grid Power: %.1f W\n", meas.PowerW)

    // Get battery status
    batteries, err := p1.GetBatteries()
    if err != nil {
        panic(err)
    }
    fmt.Printf("Battery Mode: %s\n", batteries.Mode)
    fmt.Printf("Battery Power: %.1f W\n", batteries.PowerW)

    // Set battery mode
    if err := p1.SetBatteryMode("zero"); err != nil {  // "zero", "to_full", or "standby"
        panic(err)
    }
}
```

### kWh Meter (PV Monitoring)

```go
package main

import (
    "fmt"
    "time"

    "github.com/mluiten/evcc-homewizard-v2/device"
)

func main() {
    // Create kWh device
    kwh := device.NewKWHDevice("192.168.1.11", "your-token-here", 30*time.Second)
    defer kwh.Stop()

    // Start connection and wait
    if err := kwh.StartAndWait(5 * time.Second); err != nil {
        panic(err)
    }

    // Get measurements
    meas, err := kwh.GetMeasurement()
    if err != nil {
        panic(err)
    }
    fmt.Printf("Power: %.1f W\n", meas.PowerW)
    fmt.Printf("Energy Imported: %.3f kWh\n", meas.EnergyImportkWh)
    fmt.Printf("Energy Exported: %.3f kWh\n", meas.EnergyExportkWh)
}
```

### Battery (SoC Monitoring)

```go
package main

import (
    "fmt"
    "time"

    "github.com/mluiten/evcc-homewizard-v2/device"
)

func main() {
    // Create battery device
    bat := device.NewBatteryDevice("192.168.1.12", "your-token-here", 30*time.Second)
    defer bat.Stop()

    // Start connection and wait
    if err := bat.StartAndWait(5 * time.Second); err != nil {
        panic(err)
    }

    // Get measurements
    meas, err := bat.GetMeasurement()
    if err != nil {
        panic(err)
    }
    fmt.Printf("State of Charge: %.1f%%\n", meas.StateOfChargePct)
    fmt.Printf("Power: %.1f W\n", meas.PowerW)
    fmt.Printf("Charge Cycles: %d\n", meas.Cycles)
}
```

## API Reference

### Package: `device`

#### Device Types and Constants

```go
const (
    DeviceTypeP1Meter  DeviceType = "p1meter"
    DeviceTypeKWHMeter DeviceType = "kwhmeter"
    DeviceTypeBattery  DeviceType = "battery"
)

const (
    DefaultTimeout         = 30 * time.Second
    DefaultMaxCharge       = 800.0 // W
    DefaultMaxDischarge    = 800.0 // W
    DefaultBatteryCapacity = 2.47  // kWh
)
```

#### P1 Device

```go
func NewP1Device(host, token string, timeout time.Duration) *P1Device
func (d *P1Device) Start(errC chan error)
func (d *P1Device) StartAndWait(timeout time.Duration) error  // Convenience method
func (d *P1Device) Stop()
func (d *P1Device) GetMeasurement() (P1Measurement, error)
func (d *P1Device) GetBatteries() (BatteriesData, error)
func (d *P1Device) SetBatteryMode(mode string) error  // "zero", "to_full", "standby"
```

#### kWh Device

```go
func NewKWHDevice(host, token string, timeout time.Duration) *KWHDevice
func (d *KWHDevice) Start(errC chan error)
func (d *KWHDevice) StartAndWait(timeout time.Duration) error  // Convenience method
func (d *KWHDevice) Stop()
func (d *KWHDevice) GetMeasurement() (KWHMeasurement, error)
```

#### Battery Device

```go
func NewBatteryDevice(host, token string, timeout time.Duration) *BatteryDevice
func (d *BatteryDevice) Start(errC chan error)
func (d *BatteryDevice) StartAndWait(timeout time.Duration) error  // Convenience method
func (d *BatteryDevice) Stop()
func (d *BatteryDevice) GetMeasurement() (BatteryMeasurement, error)
func (d *BatteryDevice) DefaultCapacity() float64
```

### Package: `discovery`

```go
func DiscoverDevices(ctx context.Context, onDevice func(DiscoveredDevice)) error

type DiscoveredDevice struct {
    Instance string      // Device instance name
    Host     string      // IP address or hostname
    Port     int         // Port number (usually 443)
    Type     device.DeviceType  // Device type
}
```

### Package: `pairing`

```go
// Interactive pairing tool - used by evcc token command
func Run(name string, timeout int) error
```

## Device Pairing

To obtain device tokens, use the `evcc token homewizard` command from the [evcc](https://github.com/evcc-io/evcc) project, or follow these steps:

1. Press the button on your HomeWizard device to enable pairing mode
2. Send a POST request to `https://<device-ip>/api/user` with:
   ```json
   {
     "name": "local/yourapp"
   }
   ```
   with header: `X-Api-Version: 2`
3. The device will respond with a token

## Architecture

- **WebSocket Connection**: Persistent WebSocket connection with automatic reconnection
- **Authentication**: OAuth 2.0-style token authentication via WebSocket
- **Topic Subscription**: Subscribe to "measurement" and "batteries" topics
- **Thread-Safe**: All operations are protected with proper synchronization

## Dependencies

- `github.com/coder/websocket` - WebSocket client
- `github.com/libp2p/zeroconf/v2` - mDNS discovery
- `github.com/evcc-io/evcc` - Utilities (util.Logger, util.Monitor, request.Helper)

## License

MIT License - see [LICENSE](LICENSE) file for details.

## Contributing

Contributions welcome! Please open an issue or submit a pull request.

## Development

### Building and Testing

```bash
# Build all packages
make build

# Run tests with coverage
make test

# Format code
make fmt

# Check formatting
make fmt-check

# Run all CI checks
make ci
```

### Go Version Requirement

This library requires **Go 1.23+** (go.mod specifies 1.25 due to evcc dependencies, but works with 1.23).

### CI/CD

GitHub Actions runs:
- **Tests** on Go 1.23 and stable
- **Format checks** with `gofmt`
- **Linting** with `golangci-lint`

## Related Projects

- [evcc](https://github.com/evcc-io/evcc) - EV Charge Controller (uses this library)
- [HomeWizard Energy API Documentation](https://api-documentation.homewizard.com/)

## Maintenance

This library is maintained independently from evcc to reduce the maintenance burden on the evcc core team while providing full HomeWizard Energy device support.
