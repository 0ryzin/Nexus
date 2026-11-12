# Publisher identity and code signing

## What NEXUS has today (0.4.0)

`NEXUS.exe` contains a Windows **version resource**, generated at build time by
`app/tools/release/make_rsrc.py` from `app/VERSION`:

| Field | Value |
|---|---|
| CompanyName | Nexus Development |
| ProductName | NEXUS |
| FileDescription | NEXUS |
| FileVersion / ProductVersion | the NEXUS version, e.g. 0.4.0 |
| OriginalFilename | NEXUS.exe |
| LegalCopyright | Copyright (C) Nexus Development. Third-party notices: Licences folder. |

Windows shows these values in *Properties › Details*, and Task Manager shows the description.
The CI build checks them in the packaged `NEXUS.exe`. The generator's output was compared byte
for byte with `llvm-rc`'s output for the same values.

**This is application metadata, not a signature.** Anyone can write any company name into a
version resource. Windows does not trust it and does not show it as a *verified publisher*.

## What NEXUS does not have

- **No Authenticode signature.** `NEXUS.exe`, `nexus_capture.dll` and `nexus_engine.dll` are
  unsigned. The bundled Microsoft Visual C++ runtime DLLs keep Microsoft's own signatures. NEXUS has no code-signing certificate, and none has been created, borrowed or
  imitated.
- As a result:
  - Windows SmartScreen may show *Windows protected your PC* for a newly downloaded copy;
  - a UAC prompt, if any, would show *Unknown publisher*;
  - the *Digital Signatures* tab is absent.
- NEXUS does not disable, bypass or suppress SmartScreen or Microsoft Defender in any way. The
  release page tells users that the file is unsigned and gives its SHA-256, so they can check the
  download.

## How NEXUS could be signed later (investigation)

| Option | What it needs | Effect |
|---|---|---|
| **Azure Trusted Signing** (Microsoft) | An Azure subscription and identity validation of the publisher (an organisation, or an individual in supported countries). There is a monthly fee. It signs from CI with `signtool` and the Trusted Signing dlib. | A Microsoft-issued certificate in the publisher's validated name. SmartScreen reputation still builds up over downloads. |
| **OV code-signing certificate** from a CA (DigiCert, Sectigo, SSL.com …) | Organisation validation. Since 2023 the private key must be kept on a hardware token or in a cloud HSM; cloud-HSM services can sign from CI. Paid yearly. | A *verified publisher* in the name on the certificate. SmartScreen reputation builds over time. |
| **EV code-signing certificate** | Stricter validation and a hardware/cloud HSM, at a higher cost. | Historically gave immediate SmartScreen reputation. Microsoft no longer guarantees this, so it is less of an advantage now. |
| **SignPath Foundation** (free for open-source projects) | An approved open-source project with a public repository and reproducible CI builds. | Signed by the SignPath Foundation's certificate, so the publisher shown is not "Nexus Development". |

Whichever option is chosen, signing slots into `nexus-build.yml`:
1. after `make_release.py` stages the files, and before the ZIP is made;
2. sign `NEXUS.exe`, `nexus_capture.dll` and `Engine\*.dll` with an RFC 3161 timestamp;
3. in the fresh-extract check, verify them with `Get-AuthenticodeSignature`.

The certificate or its credentials would live in GitHub Actions secrets, never in the repository
or in the distributed application.

**The publisher name on a signature is the legal name the certificate was issued to.**
"Nexus Development" can appear as a verified publisher only if a certificate is issued in that
name, which needs a registered organisation (or, for some services, an individual whose name
would appear instead).
