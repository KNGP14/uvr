# CAN-Bus Kommunikation zwischen Linux und UVR1611

> [!WARNING]
> Work in progress! Do not use it for now!

## Hardware
- Raspberry Pi (getestet mit 4B und 2B unter Ubuntu)
- CAN-Bus Adapter [SBC-CAN01 von joy-it][canbusadapter]

### Installation des CAN-Bus Adapter
- Verkabelung entsprechend der [Anleitung][canbusadapterdocs]
- SPI-Kommunikation aktivieren
  ```
  sudo raspi-config
  ```
  ```
  Interface Options > SPI > Yes
  ```
- Kernel konfigruieren
  - Kernelversion identifizieren für weitere Schritte:
    ```
    uname -r
    ```
    ```
    6.6.51+rpt-rpi-v7
    >> Kernel-Version: 6.6.51
    ```
  - boot-Konfiguration anpassen:
    ```
    sudo nano /boot/firmware/config.txt
    ```
    Im Bereich [all] folgende Zeilen hinzufügen:
    - Kernel > 4.4x
      ```
      dtparam=spi=on 
      dtoverlay=mcp2515-can0,oscillator=16000000,interrupt=25 
      dtoverlay=sp1-1cs
      ```
    - Kernel < 4.4.x
      ```
      dtparam=spi=on 
      dtoverlay=mcp2515-can0,oscillator=16000000,interrupt=25 
      dtoverlay=sp1-bcm2835-overlay 
      ```
  - Raspberry neustarten:
    ```
    sudo reboot
    ```
- SOCKETCAN-Treiber laden (nicht persitent, automatisches Laden wird später eingerichtet)
  ```
  sudo modprobe can
  ```
  ```
  sudo modprobe can_raw
  ```
- CAN-Interface aktivieren:
  ```
  sudo ip link set can0 up type can bitrate 500000
  ```
- Kommunikation testen durch Lesen der CAN-Kommunikation:
  ```
  sudo apt-get install can-utils -y
  ```
  ```
  candump can0
  ```
  ```
  can0  701   [1]  05
  can0  201   [8]  CB 03 EE 01 00 00 00 00
  ...
  ```
#### Automation bei Systemstart
- Treiber
  ```
  sudo nano /etc/modules-load.d/can.conf
  ```
  ```
  can
  can_raw
  ```
- CAN-Interface
  ```
  sudo systemctl start systemd-networkd
  ```
  ```
  sudo systemctl enable systemd-networkd
  ```
  ```
  sudo nano /etc/systemd/network/80-can.network
  ```
  ```
  [Match]
  Name=can0
  [CAN]
  BitRate=50K
  RestartSec=100ms
  ```
  ```
  sudo systemctl restart systemd-networkd
  ```
## Nutzungshinweise

Mithilfe der uvrdump-Executable ([Release-Tab][releasetab]) können sämtliche Ein- und Ausgänge eines [UVR1611][uvr1611] eingelesen werden.
```
./uvrdump --help
```
```
Usage of uvrdump:
  -client_id int
        id of the client; range from [1...254] (default 16)
  -if string
        name of the can network interface (default "can0")
  -server_a_id int
        id of the server to which the client connects to: range from [1...254] (default 1)
  -server_b_id int
        id of the server to which the client connects to: range from [1...254] (default 2)
```

## Entwicklungsumgebung
Nur erforderlich, falls der Quellcode angepasst werden soll.

coming soon: Bei Verwendung eines Standard-Raspian Betriebssystems kann die kompollierte Version im [Release-Tab][releasetab] verwendet werden.

#### Installation von Go
```
sudo apt update && sudo apt install golang -y && go version
```
#### Quellcode herunterladen
```
git clone https://github.com/KNGP14/uvr && cd uvr
```
#### Projekt initialisieren
```
go mod init github.com/brutella/uvr
```
#### Installation notwendiger Go-Pakete/Bibliotheken
```
go get github.com/brutella/can
```
```
go get github.com/brutella/canopen
```
#### Quellcode kompillieren
Das kompillieren erfolgt aus dem Root-Verzeichnis des Projektes heraus, der Ordner mit der `Makefile`-Datei.
Die Versionsnummer `VERSION` kann bei jedem Kompillierungsvorgang gleich bleiben und ist nur relevant für die Aufbewahrung verschiedener Versionen (beim Testen).
```
VERSION=0.0.1 make build-uvrdump
```

## Referenzen
Der Quellcode für uvrdump ist geforked von [brutella][uvrdump] und verwendet dessen Bilbiotheken [can][can] und [canopen][canopen] für die Kommunikation über den CAN-Bus.

[can]: https://github.com/brutella/can
[canopen]: https://github.com/brutella/canopen
[uvrdump]: https://github.com/brutella/uvr
[uvr1611]: https://www.ta.co.at/fileadmin/Downloads/Betriebsanleitungen/00_Auslauftypen/UVR1611/Manual_UVR1611_A4.03-2.pdf
[canbusadapter]: https://joy-it.net/de/products/SBC-CAN01
[canbusadapterdocs]: https://joy-it.net/files/files/Produkte/SBC-CAN01/SBC-CAN01-Anleitung-20201021.pdf
[releasetab]: https://github.com/KNGP14/uvr/releases
