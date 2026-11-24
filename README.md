# LanBeam

Send files between your **iPhone and Windows PC** over Wi-Fi. Fast, private, and nothing to install on the phone.

1. Run `lanbeam.exe` on your PC.
2. Scan the QR code with your iPhone camera.
3. Send photos, videos and files either way, at full Wi-Fi speed.

Files never leave your network. No cloud, no account, no cable, no iTunes.

## Download

Get the latest build from **[Releases](https://github.com/wylan24/lanbeam/releases/latest)**:

| System | File |
| --- | --- |
| Windows 10/11 (most PCs) | `lanbeam-windows-amd64.exe` |
| Windows on ARM | `lanbeam-windows-arm64.exe` |
| macOS (Apple silicon / Intel) | `lanbeam-macos-arm64` / `lanbeam-macos-amd64` |
| Linux | `lanbeam-linux-amd64` |

It is a single file of about 10 MB with no installer and no dependencies.

## Using it

**On the PC:** double-click `lanbeam.exe`. A console window shows a QR code, and the LanBeam page opens in your browser with the same code.

> The first time you run it, Windows Firewall asks whether LanBeam may use the network. Tick **Private networks** and click **Allow**, otherwise the phone can't connect.

**On the iPhone:** open the Camera, point it at the QR code and tap the link. Safari opens LanBeam.

- **iPhone → PC:** tap **Choose photos or files** and pick from Photos, Camera or Files. They land in `Downloads\LanBeam` on the PC.
- **PC → iPhone:** drag files onto the LanBeam page on the PC (or click **Choose files**). They appear on the phone instantly. Tap **Save** to put photos and videos in your Photos library, or **Download** to save to Files.

Tip: in Safari, tap Share → **Add to Home Screen**. The access key is remembered between runs, so the icon keeps working whenever LanBeam is running on the PC.

### Options

```
lanbeam.exe -dir D:\Transfers     # store files somewhere else
lanbeam.exe -port 9000            # use a different port (default 8765)
lanbeam.exe -new-token            # revoke access: phones must scan again
lanbeam.exe -no-browser           # don't open the page on the PC
```

## Why it works this way

**Why a web page on the phone and not an iPhone app?** Safari can upload and download files at full speed, and with a page there is nothing to install or approve on the phone. An App Store app would add install friction and nothing in speed.

**Why not Wi-Fi Direct?** iOS doesn't let third-party apps or websites use Wi-Fi Direct. Apple keeps its peer-to-peer Wi-Fi (AWDL) for AirDrop, and that only talks to Apple devices. A plain Wi-Fi connection is the fastest link an iPhone and a Windows PC can both use. On a modern router that's usually 30 to 100+ MB/s.

**No shared Wi-Fi? Make a direct link instead.** Either of these gives you a direct phone-to-PC network without a router:

- **iPhone hotspot:** turn on *Settings → Personal Hotspot* and connect the PC to it. LanBeam picks up the hotspot address automatically.
- **PC hotspot:** turn on *Windows Settings → Network & internet → Mobile hotspot* and join that network from the iPhone. This is the closest you can get to Wi-Fi Direct on iPhone.

Restart LanBeam after switching networks so the QR code shows the new address.

## Speed tips

- Use the 5 GHz band if your router has one, and stay near it.
- A PC plugged into the router by Ethernet removes one wireless hop and is often about twice as fast.
- Keep LanBeam open in Safari while it sends. The page keeps the screen awake during transfers.

## Security

- Every link carries a random 96-bit access key (in the QR code). Without it, the server refuses all requests, so other people on the same Wi-Fi can't see or send files.
- Uploaded names are cleaned so files can't escape the LanBeam folder.
- Files opened in the browser are sandboxed so they can't run scripts.
- Traffic is plain HTTP on your local network. Don't use it on Wi-Fi you don't trust, such as a café. Run `-new-token` to revoke every link you have shared.

## Troubleshooting

| Problem | Fix |
| --- | --- |
| Phone says "can't connect" | Make sure both devices are on the same network. Check Windows Firewall allows LanBeam on **Private** networks. If your Wi-Fi is marked *Public* in Windows, switch it to *Private* or allow Public too. |
| Wrong address in the QR code | VPNs and virtual adapters can confuse it. Choose another address from the drop-down on the PC page. |
| "Scan the QR code" page | The access key is missing or was rotated. Scan the code again. |
| Guest Wi-Fi | Many guest networks isolate devices from each other. Use your main network or a hotspot. |

## Building from source

Requires [Go](https://go.dev/dl/) 1.22 or later.

```
go test ./...
go build -o lanbeam .
GOOS=windows GOARCH=amd64 go build -ldflags "-s -w" -o lanbeam.exe .
```

Pushing a tag such as `v1.2.3` builds every platform and publishes a GitHub release.

## License

MIT
