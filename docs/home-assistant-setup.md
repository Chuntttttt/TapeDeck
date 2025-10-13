# Home Assistant Setup Guide

This document explains how to set up Home Assistant for TapeDeck, including the Apple TV integration and NFC reader configuration.

## Required Integrations

### 1. Apple TV Integration

TapeDeck uses Home Assistant's Apple TV integration to control playback on your Apple TV.

**Installation**:
1. Open Home Assistant
2. Go to **Settings → Devices & Services**
3. Click **+ Add Integration**
4. Search for "Apple TV"
5. Follow the pairing instructions

**Finding Your Entity ID**:
1. Go to **Settings → Devices & Services**
2. Click on your Apple TV integration
3. Look for the `media_player.*` entity (e.g., `media_player.great_room`)
4. Copy the entity ID - you'll need it for TapeDeck configuration

**Update TapeDeck Configuration**:
```bash
# In your .env file:
APPLE_TV_ENTITY=media_player.great_room
```

### 2. ESPHome NFC Reader

TapeDeck listens for NFC tag scans via Home Assistant's WebSocket API.

**Requirements**:
- ESPHome device with RC522 or PN532 NFC reader
- Device must send `tag_scanned` events to Home Assistant

**Example ESPHome Configuration**:
```yaml
esphome:
  name: nfc-reader

# ... WiFi, API, OTA config ...

spi:
  clk_pin: GPIO18
  miso_pin: GPIO19
  mosi_pin: GPIO23

rc522_spi:
  cs_pin: GPIO5
  update_interval: 1s
  on_tag:
    - homeassistant.tag:
        tag_id: !lambda 'return x;'
```

**Verify Events Are Working**:
1. Go to **Developer Tools → Events** in Home Assistant
2. Click "Start Listening" (leave event type blank or use `*`)
3. Tap an NFC card on your reader
4. You should see a `tag_scanned` event with:
   ```json
   {
     "event_type": "tag_scanned",
     "data": {
       "tag_id": "04-16-5C-D4-2E-61-80",
       "device_id": "..."
     }
   }
   ```

If you don't see events, check your ESPHome configuration and ensure the device is properly connected to Home Assistant.

## Home Assistant API Token

TapeDeck needs a long-lived access token to communicate with Home Assistant.

**Create Token**:
1. Open Home Assistant at your HA_URL (e.g., http://10.0.0.49:8123)
2. Click your **profile** (bottom left)
3. Scroll to **Long-Lived Access Tokens**
4. Click **Create Token**
5. Name it "TapeDeck"
6. Copy the token (starts with `eyJ...`)
7. Store it immediately - you can't view it again!

**Update TapeDeck Configuration**:
```bash
# In your .env file:
HA_URL=http://10.0.0.49:8123
HA_TOKEN=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.YOUR_TOKEN_HERE...
```

**Token Permissions**:
The token has full access to Home Assistant. TapeDeck uses it to:
- Connect to WebSocket API (listen for NFC scans)
- Call `media_player.play_media` service (trigger playback)

## Media Playback Flow

When an NFC card is scanned, here's what happens:

### 1. Pairing Mode (Website Active)
If a user is on the TapeDeck website in pairing mode:
1. NFC reader sends `tag_scanned` event to Home Assistant
2. Home Assistant forwards event to TapeDeck via WebSocket
3. TapeDeck creates a new card mapping in the database
4. User sees success message on website

### 2. Playback Mode (No Active Pairing)
If no one is in pairing mode:
1. NFC reader sends `tag_scanned` event to Home Assistant
2. Home Assistant forwards event to TapeDeck via WebSocket
3. TapeDeck looks up the card mapping in database
4. TapeDeck calls Home Assistant's `media_player.play_media` service
5. Home Assistant sends command to Apple TV
6. Apple TV launches Plex app and starts playing media

## Plex Deep Links

TapeDeck uses Plex deep link URLs to tell the Apple TV which media to play.

**Format**:
```
plex://play/?metadataKey=/library/metadata/{ratingKey}&server={serverID}
```

**Example**:
```
plex://play/?metadataKey=/library/metadata/12345&server=3bca134a11ac79d97e21e787e1d3e6e4e4be3643
```

**Parameters**:
- `metadataKey`: The Plex rating key for the media item (stored in card mappings)
- `server`: Your Plex server ID (from `PLEX_SERVER_ID` in `.env`)

## Home Assistant Service Call

TapeDeck makes HTTP POST requests to Home Assistant's REST API to trigger playback.

**Endpoint**:
```
POST {HA_URL}/api/services/media_player/play_media
```

**Headers**:
```
Authorization: Bearer {HA_TOKEN}
Content-Type: application/json
```

**Body**:
```json
{
  "entity_id": "media_player.great_room",
  "media_content_type": "url",
  "media_content_id": "plex://play/?metadataKey=/library/metadata/12345&server=abc123"
}
```

**Response** (Success):
```json
[
  {
    "context": {
      "id": "...",
      "parent_id": null,
      "user_id": null
    }
  }
]
```

## Troubleshooting

### "Authentication failed: invalid token"
- Verify token is correct in `.env`
- Create a new token if the old one expired or was deleted
- Check token has no extra spaces or newlines

### "Tag scanned but nothing happens"
1. Check TapeDeck logs for "Tag scanned: {tag_id}"
2. If missing, verify ESPHome is sending events (check HA Developer Tools → Events)
3. Check WebSocket connection: logs should show "Connected to Home Assistant WebSocket"

### "Mapping created but media doesn't play"
- Check Apple TV entity ID is correct (`APPLE_TV_ENTITY`)
- Verify Apple TV integration is working in Home Assistant
- Test manually: Developer Tools → Services → `media_player.play_media`
- Check Apple TV is powered on and connected to network
- Verify Plex app is installed on Apple TV

### "Apple TV not found"
- Ensure Apple TV integration is installed in Home Assistant
- Check your entity ID: Settings → Devices & Services → Apple TV
- Update `APPLE_TV_ENTITY` in `.env` with correct entity ID

### Plex Deep Link Not Working
- Verify Plex server ID is correct (`PLEX_SERVER_ID`)
- Test URL format: `plex://play/?metadataKey=/library/metadata/{ratingKey}&server={serverID}`
- Check Plex app is signed in on Apple TV
- Try opening the deep link manually in Safari on iOS to verify format

## Configuration Summary

All required configuration in `.env`:

```bash
# Home Assistant
HA_URL=http://10.0.0.49:8123
HA_TOKEN=eyJhbGciOiJI...  # Long-lived access token

# Apple TV
APPLE_TV_ENTITY=media_player.great_room  # Your media player entity ID

# Plex
PLEX_URL=http://10.0.0.124:32400
PLEX_SERVER_ID=3bca134a11ac79d97e21e787e1d3e6e4e4be3643
```

## Future: Admin Interface

Currently, all configuration is managed via the `.env` file. In a future release (Stage 9), TapeDeck will include an admin web interface to:

- Update Home Assistant URL and token
- Select Apple TV entity from dropdown (auto-discovered from HA)
- Update Plex server URL and ID
- Test connections and validate settings
- Rotate tokens without editing files

See roadmap Stage 9 for details.

## Resources

- [Home Assistant Apple TV Integration](https://www.home-assistant.io/integrations/apple_tv/)
- [Home Assistant REST API](https://developers.home-assistant.io/docs/api/rest)
- [Home Assistant WebSocket API](https://developers.home-assistant.io/docs/api/websocket)
- [ESPHome RC522 Component](https://esphome.io/components/binary_sensor/rc522.html)
- [Plex Deep Link Format](https://support.plex.tv/articles/226568427-plex-deep-linking/)
