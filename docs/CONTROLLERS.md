# Controllers: names, button prompts, connection

## Which controller it is

NEXUS names a controller from its USB/Bluetooth **vendor and product IDs** (`app/padmodels.go`),
which identify the model. SDL's controller type comes second, and a generic name is used when
neither tells more.

| IDs (vendor:product) | Name shown | Prompts |
|---|---|---|
| 045E:0B12, 045E:0B13 | Xbox Series X\|S Controller | Xbox |
| 045E:02D1, 02DD | Xbox One Controller | Xbox |
| 045E:02E0, 02EA, 02FD, 0B20 | Xbox One S Controller | Xbox |
| 045E:02E3 / 0B00, 0B05, 0B22 | Xbox Elite Controller / Series 2 | Xbox |
| 045E:028E, 028F, 0719 | Xbox 360 Controller | Xbox |
| 054C:05C4, 09CC, 0BA0 | PlayStation 4 Controller (DualShock 4) | PlayStation |
| 054C:0CE6 / 0DF2 | PlayStation 5 Controller (DualSense / DualSense Edge) | PlayStation |
| 057E:2009 | Nintendo Switch Pro Controller | Nintendo |
| 057E:2006, 2007 | Joy-Con (L), Joy-Con (R) | Nintendo |
| 28DE:1205, 1206 | Steam Deck controls | Steam |

- **Series X and Series S.** Both consoles ship the same controller (model 1914), with the same
  IDs, so nothing can tell them apart. It is named "Xbox Series X|S Controller".
- **"Xbox 360 Controller".** The browser's Gamepad API, which drives the NEXUS menus, calls
  *every* XInput controller "Xbox 360 Controller" and gives no IDs. NEXUS therefore takes the name
  from the engine's detection (SDL). One XInput pad and one Xbox controller in the engine are
  matched. Several are named only when they are all the same model; otherwise "Xbox controller"
  (`web/pads.js`, `identifyGamepads`).
- **The 17 Nov report** ("Series X shown as Xbox 360") came from the menus showing the Gamepad
  API's name. The PC's `controllers.json` shows the engine saw `045E:0B12`, "Xbox Series X
  Controller", type `xboxone`.

## Button prompts

Prompts are SVG glyphs drawn by `padGlyphSVG` (`web/pads.js`), in the interface's text colour.
They use no emoji and no logos. They follow the **active controller**, which is the one used
last, else the chosen menu controller, else the first:

- **Xbox:** A B X Y, LB RB LT RT, View, Menu.
- **PlayStation:** Cross Circle Square Triangle (drawn as shapes), L1 R1 L2 R2, Create (PS5) or
  Share (PS4), Options, Touchpad.
- **Nintendo:** B A Y X by position, L R ZL ZR, − +.
- **Generic:** A B X Y, L1 R1 L2 R2, Select, Start.
- **D-pad:** a cross with the pressed arm filled.
- **Home / guide:** a house in a ring, labelled Xbox, PS or Home in its title.

### Mapping matches the prompt

The menus read the Gamepad API's standard mapping by *position*:

- index 0 is the bottom face button (A, Cross, Nintendo B);
- index 1 is the right one (B, Circle, Nintendo A).

A prompt for index *n* draws the button that *is* index *n* on that controller. A Cross prompt
therefore always means the bottom button, the one NEXUS reads for it. The *Confirm button*
setting names its choices with the active controller's buttons. Remapped actions show the glyph
of the button they were mapped to.

Keyboard and mouse prompts are unchanged.

## Connection

- **Hot-plug.** Controllers are noticed through the browser's `gamepadconnected` and
  `gamepaddisconnected` events. They need no restart and no extra polling. The engine's list is
  asked again only when the set of controllers changes. The toast names the model.
- **Disconnect.** The controller's held and repeating buttons are forgotten.
- **Reconnect.** Buttons already held when a controller comes back count only after they are
  released, so there is no duplicate or stuck press. Mappings are kept per user, by action.
- **Controller for NEXUS menus** (*Controllers*). "Any controller", or one controller chosen by
  its identity (the engine's serial- or GUID-based key). The choice is kept per user across
  restarts, with the controller's name, so a disconnected choice still reads "Xbox Series X|S
  Controller (not connected…)". While it is away, any controller works, so nobody is locked out.
- **In games**, players are assigned by the engine as before (*Use as Player 1*, *Add as next
  player*).

## Tested

**Tested** on Linux with Chrome and scripted gamepads (`tools/uitest/controller_prompts.py`,
`pads_test.js`; Go: `padmodels_test.go`):

- an XInput pad named from the engine;
- Xbox, then DualSense, then back to Xbox prompts as the active pad changes;
- a held button on reconnect does nothing;
- the menu controller is kept after a reload.

**Not tested with physical controllers.** That includes Bluetooth versus USB, and the
Series X|S name on the user's PC (expected from its `controllers.json` IDs).
