NEXUS for Linux
===============

One download per processor type:
  amd64  every AMD64 (x86_64) Linux PC, and the Steam Deck (the same build)
  arm64  ARM64 (aarch64) Linux computers

Two kinds of file, with the same NEXUS inside:

  NEXUS-<version>-Linux-<arch>.AppImage
    Everything included. Make it executable and run it:
      chmod +x NEXUS-*.AppImage && ./NEXUS-*.AppImage
    Recommended for the Steam Deck and most PCs.

  NEXUS-<version>-Linux-<arch>.tar.gz
    The NEXUS folder. Extract it anywhere and run ./nexus from inside it.
    Needs WebKitGTK 4.1 and GTK 3 from your distribution:
      Debian/Ubuntu: sudo apt install libwebkit2gtk-4.1-0
      Fedora:        sudo dnf install webkit2gtk4.1
      Arch:          sudo pacman -S webkit2gtk-4.1

Requirements: glibc 2.39 or newer (Ubuntu 24.04, Fedora 40, SteamOS 3.6 and later), a
Vulkan driver for games (Mesa RADV/ANV or the NVIDIA driver).

Your data
  Users, saves and settings are kept in ~/.local/share/NEXUS (or $XDG_DATA_HOME/NEXUS), so a new
  NEXUS version never touches them. Put games in ~/.local/share/NEXUS/Games with the AppImage,
  or in the Games folder next to ./nexus with the tar.gz; or add any folder in Settings.
  A file named portable.txt next to ./nexus keeps everything inside the NEXUS folder.

Steam Deck and Steam (Gaming Mode)
  1. In Desktop Mode, put the AppImage somewhere permanent (e.g. ~/Applications), make it
     executable (right-click > Properties > Permissions > Is executable).
  2. Right-click it > Add to Steam (or in Steam: Games > Add a Non-Steam Game to My Library).
  3. Return to Gaming Mode and start NEXUS from your library. It fills the screen; games open
     on top and NEXUS comes back when you close them.
  Controllers: Steam Input presents your controller to NEXUS as a standard gamepad. Keep Steam
  Input on for NEXUS (the default) so the Deck's own controls work.

Nintendo Switch 2 Pro Controller (USB) - optional, experimental on Linux
  Over a USB cable the NEXUS engine can use the Switch 2 Pro Controller directly. Linux only
  lets programs open the controller's USB device after you allow it once. NEXUS never changes
  your system by itself; to allow it, run in a terminal (asks for your password):
    sudo cp 72-nexus-switch2.rules /etc/udev/rules.d/ && sudo udevadm control --reload && sudo udevadm trigger
  then unplug and replug the controller. (In the AppImage the file is inside; the tar.gz has it
  next to this README.) To undo: sudo rm /etc/udev/rules.d/72-nexus-switch2.rules
  On the Steam Deck in Gaming Mode, Steam Input already handles controllers; you don't need this.
  Switch 2 controllers over Bluetooth and Joy-Con 2 are not supported by NEXUS on Linux.

Updates
  NEXUS tells you when a new version is out; download the new AppImage or tar.gz from the
  release page. Your data stays where it is.
