# Tubi Bridge

A lightweight Windows background service that bridges live Tubi TV streams into an M3U playlist and XMLTV guide for media servers like Channels DVR.

---

**Features**

* Automatically scrapes and formats live Tubi TV channels into an M3U playlist.
* Provides XMLTV guide data with integrated Gracenote/TMS station ID mapping.
* Runs silently in the background as a native Windows Service.
* Includes a web dashboard for copying playlist links and toggling guide settings.

---

**Installation**

* Navigate to the **Releases** page on GitHub.
* Download the latest installer executable (`TubiBridgeSetup.exe`).
* Run `TubiBridgeSetup.exe` and approve the administrator prompt (UAC) when prompted.
* Complete the setup wizard (the installer automatically registers the Windows Service, configures firewall permissions, and places a shortcut on your desktop).
* Once the installer finishes, your default browser will open the dashboard at `http://localhost:7778`.

---

**Setup & Usage**

* Open your browser and navigate to `http://localhost:7778` (or use the desktop shortcut).
* If port `7778` was busy, the application automatically increments to the next open port (e.g., `7779`) and saves it to `settings.json`. Check the dashboard header for your active port.
* Copy the **M3U Playlist URL** (`http://<YOUR_IP>:7778/playlist.m3u`).
* Copy the **XMLTV Guide URL** (`http://<YOUR_IP>:7778/epg.xml`).
* Add the M3U and XMLTV links as a custom tuner/source in your DVR software (such as Channels DVR).
* Use the checkbox on the dashboard to enable or disable Gracenote station IDs based on your guide provider preference.

---

**Managing the Service**

* The bridge starts automatically with Windows under the service name `TubiBridge`.
* To start or stop the service manually, open **Command Prompt** (Run as Administrator) and run:
  * Stop: `sc stop TubiBridge`
  * Start: `sc start TubiBridge`
* To uninstall, use Windows **Settings > Apps > Installed apps** and select **Tubi Bridge** to remove the service, files, and firewall rules cleanly.
