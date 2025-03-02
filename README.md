# CAN-Bus Kommunikation zwischen Linux und UVR1611

> [!WARNING]
> Work in progress! Do not use it for now!

## Hardware
- Raspberry Pi (getestet mit 4B und 2B)
- CAN-Bus Adapter [SBC-CAN01 von joy-it][canbusadapter]

### Installation des CAN-Bux Adapter
- Verkabelung entsprechend der [Anleitung][canbusadapterdocs]
- SOCKETCAN-Treiber einrichten
  ```
  sudo apt-get install can-utils -y
  ```
  Kernelversion identifizieren für weitere Schritte:
  ```
  uname -a
  ```
  boot-Konfiguration anpassen:
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
  Raspberry neustarten:
  ```
  sudo reboot
  ```
- CAN-Interface aktivieren:
  ```
  sudo ip link set can0 up type can bitrate 500000
  ```
- Kommunikation testen durch Lesen der CAN-Kommunikation:
  ```
  candump can0
  ```
  ```
  can0  701   [1]  05
  can0  201   [8]  CB 03 EE 01 00 00 00 00
  ...
  ```

## Nutzungshinweise

Mithilfe der uvrdump-Executable ([Release-Tab][releasetab]) können sämtliche Ein- und Ausgänge eines [UVR1611][uvr1611] eingelesen werden.
```
./uvrdump --help
....
```

## Entwicklungsumgebung
Nur erforderlich, falls der Quellcode angepasst werden soll.

coming soon: Bei Verwendung eines Standard-Raspian Betriebssystems kann die kompollierte Version im [Release-Tab][releasetab] verwendet werden.

#### Installation von Go
```
sudo apt update && sudo apt install golang -y && go version
```
#### Installation notwendiger Go-Pakete/Bibliotheken
```
go get github.com/brutella/can
```
```
go get github.com/brutella/canopen
```
#### Quellcode herunterladen
```
git clone https://github.com/KNGP14/uvr && cd uvr
```
#### Quellcode kompillieren
Das kompllieren erfolgt aus dem Root-Verzeichnis des Projektes heraus, der Ordner mit der `Makefile`-Datei.
```
tbd
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
