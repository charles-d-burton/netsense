# NetSense

NetSense is a lightweight, configurable network traffic monitor written in Go. It listens for specific network traffic patterns (defined by IP ranges, ports, and protocols) and publishes state changes to an MQTT broker.

Originally designed to detect active video conferencing calls (like Zoom or Teams) to automate home automation tasks (e.g., turning on "On Air" lights), it has been generalized to trigger events based on any defined network activity.

## Features

- **Real-time Traffic Monitoring:** Uses `libpcap` to efficiently capture and filter packets.
- **Flexible Filtering:** Define rules based on Service, CIDR blocks, Port ranges, and Protocols.
- **MQTT Integration:** Publishes state (`active`/`inactive`) and Home Assistant auto-discovery configurations.
- **Resilient:** Handles network reconnections and graceful shutdowns.
- **Cross-Platform:** Runs on Linux, Windows, and macOS (requires pcap drivers).

## Prerequisites

- **Go 1.25** or higher.
- **libpcap** development headers:
  - **Ubuntu/Debian:** `sudo apt-get install libpcap-dev`
  - **Fedora/RHEL:** `sudo dnf install libpcap-devel`
  - **Windows:** Install [Npcap](https://npcap.com/) (ensure "Install Npcap in WinPcap API-compatible Mode" is selected).
  - **macOS:** `libpcap` is usually included, or install via Xcode Command Line Tools.

## Installation

1.  Clone the repository:

    ```bash
    git clone https://github.com/charles-d-burton/netsense.git
    cd netsense
    ```

2.  Build the binary:
    ```bash
    go build -o netsense .
    ```

## Usage

NetSense requires administrative privileges to capture network traffic.

### Linux (Recommended)
You can avoid running as root by granting the binary the necessary capabilities:
```bash
sudo setcap 'cap_net_raw,cap_net_admin=eip' ./netsense
./netsense
```

### macOS/Windows/Linux (Alternative)
```bash
sudo ./netsense
```

## Features Deep-Dive

### Interface Hot-swapping
NetSense periodically scans for network interface changes. If you plug in an Ethernet cable or connect to a VPN, NetSense will automatically start monitoring the new interface without a restart.

### State Debouncing
Prevent "flickering" of the sensor state during intermittent traffic by adding delays:
- `on_delay`: Traffic must be sustained for this long before state becomes `active`.
- `off_delay`: Traffic must be absent for this long before state becomes `inactive`.

## Configuration

Create a `config.yaml` file to define your MQTT settings, global device information, and traffic filters.

### Example Configuration

```yaml
mqtt:
  host: "192.168.1.50"
  port: "1883"
  user: "mqtt_user"
  password: "mqtt_password"
  client_id: "my_laptop"

device:
  name: "My Work Laptop"
  model: "NetSense Monitor"
  manufacturer: "NetSense"
  identifiers: ["work_laptop_01"]

sensors:
  - name: "Zoom"
    homeassistant:
      name: "Zoom Meeting"
      device_class: "connectivity"
      state_topic: "homeassistant/binary_sensor/zoom/state"
      availability_topic: "homeassistant/binary_sensor/zoom/availability"
      unique_id: "netsense_zoom_01"

    pcap:
      active_threshold: 5
      on_delay: 2s
      off_delay: 30s

    filters:
      - service: "Zoom Media"
        direction: "both"
        protocols: ["udp"]
        portranges: ["3478-3479", "8801-8810"]
        cidrs: ["3.235.64.0/19", "50.202.0.0/16"] # etc...

  - name: "Teams"
    homeassistant:
      name: "Teams Meeting"
      device_class: "connectivity"
      state_topic: "homeassistant/binary_sensor/teams/state"
      availability_topic: "homeassistant/binary_sensor/teams/availability"
      unique_id: "netsense_teams_01"

    pcap:
      active_threshold: 5
      on_delay: 2s
      off_delay: 30s

    filters:
      - service: "Teams Media"
        direction: "both"
        protocols: ["udp"]
        portranges: ["3478-3481"]
```

### Configuration Options

#### `mqtt`

Connection settings for your MQTT broker.

#### `device`

Common device information shared by all sensors (Home Assistant integration).
- `name`: Human-readable device name.
- `model`: Device model identifier.
- `manufacturer`: Device manufacturer.
- `identifiers`: Unique identifiers for the physical device.

#### `sensors`

A list of sensors to monitor. Each sensor manages its own state and Home Assistant entity.

- `homeassistant`: HA entity configuration.
  - `unique_id`: MUST be unique for each sensor.
- `pcap`: Interface capture and debouncing settings.
- `filters`: Traffic matching rules.

## License

[Apache-2.0](LICENSE)
