# Hardware Setup Guide

This guide covers the physical hardware setup for TapeDeck, including NFC reader selection, wiring, ESPHome configuration, and Home Assistant integration.

## Hardware Requirements

### Required Components

1. **ESP32 or ESP8266 Microcontroller**
   - Recommended: ESP32 DevKit (better WiFi, more GPIO pins)
   - Alternative: ESP8266 NodeMCU (budget option)
   - Cost: $5-15 USD

2. **NFC Reader Module**
   - **RC522 (Recommended)**: Cheap, reliable, 13.56MHz RFID
     - Cost: $2-5 USD
     - Range: ~3cm
     - Supports ISO 14443A tags (MIFARE Classic, NTAG213/215/216)
   - **PN532**: More advanced, supports NFC peer-to-peer
     - Cost: $8-15 USD
     - Range: ~5cm
     - Supports more tag types

3. **NFC Cards/Tags**
   - **NTAG213/215/216** (Recommended): ISO 14443A, rewritable
   - **MIFARE Classic 1K**: Older standard, widely available
   - **Adhesive NFC Stickers**: For attaching to toys, books, etc.
   - Cost: $0.30-1.00 USD per tag
   - Recommended quantity: 20-50 tags to start

4. **USB Power Supply**
   - 5V, 1A minimum
   - Micro-USB (ESP32/ESP8266) or USB-C (newer boards)

5. **Jumper Wires**
   - Female-to-female (if using breadboard)
   - 6-8 wires needed

### Optional Components

- **Breadboard**: For prototyping before permanent installation
- **Enclosure**: Plastic project box to house the reader
- **LED Indicator**: Visual feedback for successful scans
- **Buzzer**: Audio feedback for scans

## Wiring Guide

### RC522 to ESP32

The RC522 communicates via SPI (Serial Peripheral Interface).

| RC522 Pin | ESP32 Pin | Description |
|-----------|-----------|-------------|
| SDA/SS    | GPIO 5    | Chip Select |
| SCK       | GPIO 18   | SPI Clock |
| MOSI      | GPIO 23   | Master Out Slave In |
| MISO      | GPIO 19   | Master In Slave Out |
| IRQ       | Not connected | Interrupt (optional) |
| GND       | GND       | Ground |
| RST       | GPIO 27   | Reset (optional) |
| 3.3V      | 3.3V      | Power (3.3V only!) |

**IMPORTANT**: RC522 operates at 3.3V. Do NOT connect to 5V or you'll damage the module.

### RC522 to ESP8266

| RC522 Pin | ESP8266 Pin | Description |
|-----------|-------------|-------------|
| SDA/SS    | GPIO 15 (D8) | Chip Select |
| SCK       | GPIO 14 (D5) | SPI Clock |
| MOSI      | GPIO 13 (D7) | Master Out Slave In |
| MISO      | GPIO 12 (D6) | Master In Slave Out |
| IRQ       | Not connected | Interrupt |
| GND       | GND         | Ground |
| RST       | GPIO 0 (D3) | Reset |
| 3.3V      | 3.3V        | Power |

### PN532 Wiring (I2C Mode)

The PN532 can operate in SPI, I2C, or UART mode. I2C is simplest.

| PN532 Pin | ESP32 Pin | Description |
|-----------|-----------|-------------|
| SDA       | GPIO 21   | I2C Data |
| SCL       | GPIO 22   | I2C Clock |
| GND       | GND       | Ground |
| VCC       | 3.3V      | Power |

**Mode Selection**: Set PN532 DIP switches to I2C mode (check your module's documentation).

## ESPHome Configuration

### Initial Setup

1. **Install ESPHome**:
   ```bash
   # Home Assistant Add-on (recommended):
   # Settings → Add-ons → ESPHome

   # Or standalone:
   pip3 install esphome
   ```

2. **Create Device Configuration**:
   ```bash
   esphome wizard nfc-reader.yaml
   ```
   - Device name: `nfc-reader`
   - Platform: `ESP32` or `ESP8266`
   - Board: Select your specific board (e.g., `esp32dev`, `nodemcuv2`)
   - WiFi SSID and password

### RC522 Configuration (ESP32)

Complete `nfc-reader.yaml` for RC522 on ESP32:

```yaml
esphome:
  name: nfc-reader
  platform: ESP32
  board: esp32dev

# Enable logging
logger:
  level: DEBUG

# Enable Home Assistant API
api:
  encryption:
    key: "YOUR_ENCRYPTION_KEY_HERE"

# Enable OTA updates
ota:
  password: "YOUR_OTA_PASSWORD"

# WiFi configuration
wifi:
  ssid: "YOUR_WIFI_SSID"
  password: "YOUR_WIFI_PASSWORD"

  # Optional: Static IP
  manual_ip:
    static_ip: 192.168.1.150
    gateway: 192.168.1.1
    subnet: 255.255.255.0

# SPI bus configuration
spi:
  clk_pin: GPIO18
  miso_pin: GPIO19
  mosi_pin: GPIO23

# RC522 NFC reader
rc522_spi:
  cs_pin: GPIO5
  update_interval: 500ms
  on_tag:
    then:
      - homeassistant.tag_scanned: !lambda 'return x;'
      - logger.log:
          format: "Tag scanned: %s"
          args: ['x.c_str()']

# Optional: Status LED
status_led:
  pin:
    number: GPIO2
    inverted: true

# Optional: Buzzer for audio feedback
output:
  - platform: esp32_dac
    pin: GPIO25
    id: buzzer_output

# Optional: Buzzer beep on scan
switch:
  - platform: output
    id: buzzer
    output: buzzer_output
    on_turn_on:
      - delay: 100ms
      - switch.turn_off: buzzer
```

### PN532 Configuration (ESP32)

Complete `nfc-reader.yaml` for PN532 on ESP32:

```yaml
esphome:
  name: nfc-reader
  platform: ESP32
  board: esp32dev

logger:
  level: DEBUG

api:
  encryption:
    key: "YOUR_ENCRYPTION_KEY_HERE"

ota:
  password: "YOUR_OTA_PASSWORD"

wifi:
  ssid: "YOUR_WIFI_SSID"
  password: "YOUR_WIFI_PASSWORD"

# I2C bus configuration
i2c:
  sda: GPIO21
  scl: GPIO22
  scan: true

# PN532 NFC reader
pn532_i2c:
  update_interval: 500ms
  on_tag:
    then:
      - homeassistant.tag_scanned: !lambda 'return x;'
      - logger.log:
          format: "Tag scanned: %s"
          args: ['x.c_str()']

status_led:
  pin:
    number: GPIO2
    inverted: true
```

### Configuration Options Explained

- **`update_interval`**: How often to poll for tags (500ms = twice per second)
  - Lower = faster response, higher power consumption
  - Higher = slower response, lower power consumption

- **`on_tag`**: Actions to perform when a tag is detected
  - `homeassistant.tag_scanned`: Sends event to Home Assistant (required for TapeDeck)
  - `logger.log`: Prints tag ID to logs for debugging

- **`status_led`**: Built-in LED that blinks during scanning
  - `inverted: true` if LED is active-low (common on ESP32)

## Flashing ESPHome

### First Flash (USB)

1. **Connect ESP32 to Computer via USB**

2. **Compile and Upload**:
   ```bash
   esphome run nfc-reader.yaml
   ```

   Or via Home Assistant ESPHome add-on:
   - Upload `nfc-reader.yaml`
   - Click **Install**
   - Select **Plug into this computer**

3. **Monitor Logs**:
   ```bash
   esphome logs nfc-reader.yaml
   ```

   Look for:
   - WiFi connection established
   - Home Assistant API connected
   - RC522/PN532 initialization successful

### Subsequent Updates (OTA)

After initial flash, updates can be done wirelessly:

```bash
esphome run nfc-reader.yaml
# Select "Over The Air" option
```

## Home Assistant Integration

### 1. Add ESPHome Device

After flashing, the device should auto-discover in Home Assistant:

1. Go to **Settings → Devices & Services**
2. Look for **Discovered: ESPHome - nfc-reader**
3. Click **Configure**
4. Enter encryption key from YAML config

### 2. Verify Tag Scanning

1. Go to **Developer Tools → Events**
2. Click **Start Listening** (leave event type field empty)
3. Tap an NFC card on the reader
4. Look for `tag_scanned` event:

```json
{
  "event_type": "tag_scanned",
  "data": {
    "tag_id": "04-1A-5B-C2-3D-7E-80",
    "device_id": "abc123..."
  },
  "origin": "LOCAL",
  "time_fired": "2025-10-14T12:34:56.789Z"
}
```

If you see this, the hardware is working correctly!

### 3. Test with TapeDeck

1. Start TapeDeck application
2. Check logs for: `Connected to Home Assistant WebSocket`
3. Scan a tag - TapeDeck logs should show: `Tag scanned: 04-1A-5B-C2-3D-7E-80`

## Troubleshooting

### RC522 Not Detected

**Symptoms**: ESPHome logs show "RC522 initialization failed"

**Solutions**:
1. Check wiring - ensure SPI pins match your configuration
2. Verify 3.3V power (measure with multimeter)
3. Try different CS pin (some ESP32 boards have conflicts)
4. Test with known-good RC522 module (they're cheap, buy spares)
5. Check for shorts/cold solder joints on module

**Common Mistakes**:
- Connected to 5V instead of 3.3V
- MOSI/MISO swapped
- CS pin conflicts with built-in flash/LED

### Tags Not Scanning

**Symptoms**: RC522 detected but no tags read

**Solutions**:
1. Test with multiple tags (one might be defective)
2. Move tag closer - maximum range is ~3cm
3. Check tag type compatibility:
   - RC522: ISO 14443A (MIFARE Classic, NTAG)
   - PN532: ISO 14443A/B, FeliCa
4. Verify `update_interval` isn't too long (try 500ms)
5. Check ESPHome logs for tag detection attempts

### Intermittent Scanning

**Symptoms**: Tags sometimes work, sometimes don't

**Solutions**:
1. **Power supply**: Use quality 1A+ power supply
   - Weak power causes brownouts during WiFi transmission
   - Add 100-1000µF capacitor across VCC/GND
2. **WiFi signal**: Move closer to router or use WiFi extender
3. **Interference**: Keep away from metal objects, other 13.56MHz devices
4. **Physical issues**: Secure all wire connections

### ESP32 Won't Flash

**Symptoms**: Upload fails or device not detected

**Solutions**:
1. Hold **BOOT** button while plugging in USB
2. Try different USB cable (data, not just power)
3. Install USB-to-serial drivers:
   - CP2102: https://www.silabs.com/developers/usb-to-uart-bridge-vcp-drivers
   - CH340: https://sparks.gogo.co.nz/ch340.html
4. Check Device Manager (Windows) or `ls /dev/tty*` (Linux/Mac)

### Tags Detected but No HA Events

**Symptoms**: ESPHome logs show tags but no events in HA

**Solutions**:
1. Verify `homeassistant.tag_scanned` is in `on_tag` config
2. Check Home Assistant API is connected (ESPHome logs)
3. Restart Home Assistant
4. Re-add ESPHome integration if needed
5. Check Home Assistant logs for errors

## Physical Installation

### Mounting Options

1. **Desktop Reader**:
   - Place RC522 module on desk
   - 3D-print enclosure or use project box
   - Route USB power cable neatly

2. **Wall-Mounted**:
   - Mount enclosure near TV/media center
   - Use adhesive tape or screws
   - Consider cable management

3. **Furniture Integration**:
   - Mount inside cabinet door
   - Embed in shelf
   - Hide in toy storage box

### Optimal Placement

- **Near TV/Entertainment Center**: Kids can scan and watch immediately
- **Child Height**: Low enough for kids to reach independently
- **Good WiFi Signal**: Ensure reliable connection to HA
- **Away from Metal**: Metal interferes with NFC signal
- **Visible LED**: Kids can see scan feedback

## Advanced Configuration

### Multiple Readers

You can run multiple NFC readers in different rooms:

```yaml
# Living room reader
esphome:
  name: nfc-reader-living-room

# Bedroom reader
esphome:
  name: nfc-reader-bedroom
```

TapeDeck receives all tag scans regardless of reader location. Configure different Apple TVs per room in the setup wizard.

### Home Assistant Automations

Example automation to flash lights when a card is scanned:

```yaml
automation:
  - alias: "NFC Card Scanned - Flash Lights"
    trigger:
      - platform: event
        event_type: tag_scanned
    action:
      - service: light.turn_on
        target:
          entity_id: light.living_room
        data:
          brightness: 255
          flash: short
```

### Custom Scan Feedback

Add visual/audio feedback in ESPHome config:

```yaml
rc522_spi:
  cs_pin: GPIO5
  update_interval: 500ms
  on_tag:
    then:
      # Send to Home Assistant
      - homeassistant.tag_scanned: !lambda 'return x;'

      # Flash LED
      - output.turn_on: scan_led
      - delay: 200ms
      - output.turn_off: scan_led

      # Beep buzzer
      - output.turn_on: buzzer
      - delay: 100ms
      - output.turn_off: buzzer

output:
  - platform: gpio
    pin: GPIO26
    id: scan_led

  - platform: gpio
    pin: GPIO27
    id: buzzer
```

## Shopping List

Budget setup (~$15):
- ESP32 DevKit: $6
- RC522 Module: $3
- 20x NTAG213 tags: $4
- Jumper wires: $2

Premium setup (~$35):
- ESP32 DevKit: $6
- PN532 Module: $12
- 50x NTAG215 tags: $10
- Custom enclosure: $5
- LED + Buzzer: $2

## Recommended Retailers

- **AliExpress**: Cheapest, 2-4 week shipping
- **Amazon**: Fast shipping, higher prices
- **Adafruit**: Quality components, great documentation
- **SparkFun**: Educational resources included

## Next Steps

After hardware is working:

1. [Complete TapeDeck Setup Wizard](../README.md#4-complete-setup-wizard)
2. [Pair Your First Card](../README.md#pairing-cards)
3. [Configure Home Assistant Integration](./home-assistant-setup.md)

## Additional Resources

- [ESPHome RC522 Component](https://esphome.io/components/binary_sensor/rc522.html)
- [ESPHome PN532 Component](https://esphome.io/components/binary_sensor/pn532.html)
- [Home Assistant ESPHome Integration](https://www.home-assistant.io/integrations/esphome/)
- [NFC Tag Types Explained](https://www.rfidhy.com/nfc-tag-types/)
- [ESP32 Pinout Reference](https://randomnerdtutorials.com/esp32-pinout-reference-gpios/)
