# CAN-Bus Kommunikation zwischen Linux und UVR1611

## Hardware
- Raspberry Pi (getestet mit 4B und 2B)
- CAN-Bus Adapter [SBC-CAN01 von joy-it][canbusadapter]

## Entwicklungsumgebung
Nur erforderlich, falls der Quellcode angepasst werden soll.

coming soon: Bei Verwendung eines Standard-Raspian Betriebssystems kann die kompollierte Version im Releases-Tab verwendet werden.

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
## Nutzung

Mithilfe der uvrdump-Executable können sämtliche Ein- und Ausgänge eines [UVR1611][uvr1611] eingelesen werden.
```
./uvrdump --help
....
```

## Referenzen
Der Quellcode für uvrdump ist geforked von [brutella][uvrdump] und verwendet dessen Bilbiotheken [can][can] und [canopen][canopen] für die Kommunikation über den CAN-Bus.

[can]: https://github.com/brutella/can
[canopen]: https://github.com/brutella/canopen
[uvrdump]: https://github.com/brutella/uvr
[uvr1611]: https://www.ta.co.at/fileadmin/Downloads/Betriebsanleitungen/00_Auslauftypen/UVR1611/Manual_UVR1611_A4.03-2.pdf
[canbusadapter]: https://joy-it.net/de/products/SBC-CAN01
