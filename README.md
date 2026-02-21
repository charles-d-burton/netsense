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

### Process Monitoring
In addition to packet capture, NetSense can monitor specific processes. If a process in your list is running, the state is considered `active`. This is platform-agnostic.

## Configuration

Create a `config.yaml` file to define your MQTT settings and traffic filters.

### Example Configuration

```yaml
# ... (Home Assistant and MQTT config)

mqtt:
  # ...
  payload_active: "ON"   # Optional, default: "ON"
  payload_inactive: "OFF" # Optional, default: "OFF"

pcap:
  interfaces: ["eth0", "wlan0"] # Optional, default: all non-loopback
  active_threshold: 1          # Optional, default: 1 (packets per interval)
  on_delay: 5s                 # Optional, default: 0s
  off_delay: 30s                # Optional, default: 0s

process:
  enabled: true
  processes:
    - "Zoom"
    - "Teams"
    - "slack"

filters:
# ...
```

### Configuration Options

#### `homeassistant`

Defines the entity that will appear in Home Assistant.

- `name`: Display name of the sensor.
- `device_class`: UI icon/treatment (e.g., `connectivity`, `presence`, `lock`).
- `state_topic`: MQTT topic where `active` or `inactive` payloads are sent.
- `unique_id`: Unique identifier for the sensor.

#### `mqtt`

Connection settings for your MQTT broker.

- `host`: Broker IP or hostname.
- `port`: Broker port (usually 1883).
- `user` / `password`: Credentials.

#### `pcap`

Packet capture tuning.

- `promiscuous`: Set to `true` to capture all traffic on the interface, not just traffic destined for the host.

#### `filters`

A list of rules to match network traffic. If **any** filter matches, the state becomes `active`.

- `cidrs`: List of destination IP ranges (CIDR notation).
- `portranges`: List of destination ports (e.g., "80", "443", "8000-8010").
- `protocols`: List of protocols (e.g., "tcp", "udp", "icmp").

**Note:** Within a single filter entry, conditions are ANDed (e.g., "IP is X AND Port is Y"). Across different filter entries, they are ORed.

## License

[MIT](LICENSE)
