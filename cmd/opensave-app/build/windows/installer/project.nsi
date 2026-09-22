Unicode true

####
## Please note: Template replacements don't work in this file. They are provided with default defines like
## mentioned underneath.
## If the keyword is not defined, "wails_tools.nsh" will populate them with the values from ProjectInfo.
## If they are defined here, "wails_tools.nsh" will not touch them. This allows to use this project.nsi manually
## from outside of Wails for debugging and development of the installer.
##
## For development first make a wails nsis build to populate the "wails_tools.nsh":
## > wails build --target windows/amd64 --nsis
## Then you can call makensis on this file with specifying the path to your binary:
## For a AMD64 only installer:
## > makensis -DARG_WAILS_AMD64_BINARY=..\..\bin\app.exe
## For a ARM64 only installer:
## > makensis -DARG_WAILS_ARM64_BINARY=..\..\bin\app.exe
## For a installer with both architectures:
## > makensis -DARG_WAILS_AMD64_BINARY=..\..\bin\app-amd64.exe -DARG_WAILS_ARM64_BINARY=..\..\bin\app-arm64.exe
####
## The following information is taken from the ProjectInfo file, but they can be overwritten here.
####
## !define INFO_PROJECTNAME    "MyProject" # Default "{{.Name}}"
## !define INFO_COMPANYNAME    "MyCompany" # Default "{{.Info.CompanyName}}"
## !define INFO_PRODUCTNAME    "MyProduct" # Default "{{.Info.ProductName}}"
## !define INFO_PRODUCTVERSION "1.0.0"     # Default "{{.Info.ProductVersion}}"
## !define INFO_COPYRIGHT      "Copyright" # Default "{{.Info.Copyright}}"
###
## !define PRODUCT_EXECUTABLE  "Application.exe"      # Default "${INFO_PROJECTNAME}.exe"
## !define UNINST_KEY_NAME     "UninstKeyInRegistry"  # Default "${INFO_COMPANYNAME}${INFO_PRODUCTNAME}"
####
## !define REQUEST_EXECUTION_LEVEL "admin"            # Default "admin"  see also https://nsis.sourceforge.io/Docs/Chapter4.html
####
## Include the wails tools
####
!include "wails_tools.nsh"

# The version information for this two must consist of 4 parts
VIProductVersion "${INFO_PRODUCTVERSION}.0"
VIFileVersion    "${INFO_PRODUCTVERSION}.0"

VIAddVersionKey "CompanyName"     "${INFO_COMPANYNAME}"
VIAddVersionKey "FileDescription" "${INFO_PRODUCTNAME} Installer"
VIAddVersionKey "ProductVersion"  "${INFO_PRODUCTVERSION}"
VIAddVersionKey "FileVersion"     "${INFO_PRODUCTVERSION}"
VIAddVersionKey "LegalCopyright"  "${INFO_COPYRIGHT}"
VIAddVersionKey "ProductName"     "${INFO_PRODUCTNAME}"

# Enable HiDPI support. https://nsis.sourceforge.io/Reference/ManifestDPIAware
ManifestDPIAware true

!include "MUI.nsh"
!include "LogicLib.nsh"
!include "FileFunc.nsh"

## The hive wails.writeUninstaller records the installation in. A constant
## in production; overridable so a test build, which cannot write HKLM
## without elevation, can still exercise the already-installed path below.
!ifndef UNINST_ROOT
    !define UNINST_ROOT HKLM
!endif

!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
!define MUI_FINISHPAGE_NOAUTOCLOSE # Wait on the INSTFILES page so the user can take a look into the details of the installation steps
!define MUI_UNFINISHPAGE_NOAUTOCLOSE
!define MUI_ABORTWARNING # This will warn the user if they exit from the installer.

####
## Let the finish page colour its own check boxes.
##
## SetCtlColors has no effect on the text of a themed check box - MUI says
## so itself, calling it bug #443 - so it asks UxTheme to stop theming those
## controls first. It only does that in high-contrast mode unless this is
## defined, which on an ordinary desktop leaves the two labels below drawn
## near-black on the dark page and effectively invisible. Defined, they take
## the colours set here. It reaches nothing else: in the whole of MUI this
## symbol is read twice, both in the guard around that one workaround.
##
## The boxes draw in the classic style as a result. A tick that looks a
## little older is a better trade than a label nobody can read.
####
!define MUI_FORCECLASSICCONTROLS

####
## The app's own look, as far as a native installer can carry it.
##
## MUI paints the header strip and the welcome and finish panels; the frame,
## the buttons and the body of the middle pages belong to Windows and are
## left alone, which is the right place to stop — an installer that repaints
## the OS chrome looks like something to be suspicious of. The two bitmaps
## are built from the app icon and these same colours by make_art.py beside
## this file, at the sizes NSIS will not scale: 164x314 and 150x57.
####
!define MUI_BGCOLOR "0C0C0D"                 ## --bg
!define MUI_TEXTCOLOR "E8E8EA"               ## --text
!define MUI_WELCOMEFINISHPAGE_BITMAP "welcome.bmp"
!define MUI_UNWELCOMEFINISHPAGE_BITMAP "welcome.bmp"
!define MUI_HEADERIMAGE
!define MUI_HEADERIMAGE_RIGHT
!define MUI_HEADERIMAGE_BITMAP "header.bmp"
!define MUI_HEADERIMAGE_UNBITMAP "header.bmp"

####
## And words that say something. The stock text explains what an installer is
## and asks the reader to close other applications, which is advice from an
## era when installers replaced system DLLs. This one closes OpenSave itself
## if it is running, so there is nothing to ask for.
##
## One paragraph each, with no line breaks inside them.
##
## These labels are created with their text baked in at compile time, and a
## break in the value truncates the label there - everything after the first
## one is silently dropped, with no warning from the compiler. The stock text
## gets away with breaks because it arrives as a language string, resolved at
## runtime. Checked on NSIS 3.12 with both $\r$\n and $\n: same result.
####
!define MUI_WELCOMEPAGE_TITLE "Install OpenSave"
!define MUI_WELCOMEPAGE_TEXT "OpenSave keeps your game saves in step across your own computers, over your network or over the internet with end-to-end encryption. Nothing passes through anyone else's servers, and your saves stay where the games put them. If OpenSave is already running, Setup closes it first."
!define MUI_FINISHPAGE_TITLE "OpenSave is installed"
!define MUI_FINISHPAGE_TEXT "Open OpenSave on another computer and pair the two to start syncing. Your snapshots and settings live in your user folder, so updating or reinstalling never disturbs them."
!define MUI_UNCONFIRMPAGE_TEXT_TOP "This removes OpenSave from this computer. Your snapshots, settings and paired devices are kept unless you choose otherwise on the next screen, and your games' own save files are never touched."

####
## Where the user's saves, snapshots, settings, pairings and keys live. The
## installer never writes here and the uninstaller only removes it if asked
## to, in as many words — see the uninstall section.
####
## Overridable so this installer can be exercised end to end - built under
## a different product name, against scratch directories - without a test
## run being able to reach a real installation or a real person's data.
!ifndef OPENSAVE_DATA_DIR
    !define OPENSAVE_DATA_DIR "$PROFILE\.opensave"
!endif
## Where `opensave install` puts the command-line tool, and the PATH entry it
## adds. The CLI is installed separately and can outlive the app; the
## uninstaller offers to take it too.
!ifndef OPENSAVE_CLI_DIR
    !define OPENSAVE_CLI_DIR "$LOCALAPPDATA\OpenSave\bin"
!endif
## The autostart entry the app writes when "start with Windows" is on, and
## which the checkbox on the finish page writes directly.
!define OPENSAVE_RUN_KEY "Software\Microsoft\Windows\CurrentVersion\Run"

####
## Finish page: two checkboxes.
##
## "Run" is the ordinary one. The second is the app's own start-with-Windows
## setting, written here as the registry entry the app uses — the installer
## cannot open the database the setting lives in, so the app adopts the entry
## on its next launch and Settings agrees with Windows. A sync tool that only
## runs when you remember to open it does not sync, so it is on by default,
## and it is a visible checkbox rather than a silent decision.
####
!define MUI_FINISHPAGE_RUN
!define MUI_FINISHPAGE_RUN_TEXT "Start OpenSave now"
!define MUI_FINISHPAGE_RUN_FUNCTION "LaunchOpenSave"
!define MUI_FINISHPAGE_SHOWREADME ""
!define MUI_FINISHPAGE_SHOWREADME_TEXT "Start OpenSave when Windows starts"
!define MUI_FINISHPAGE_SHOWREADME_FUNCTION "EnableAutostart"

!insertmacro MUI_PAGE_WELCOME # Welcome to the installer page.
# !insertmacro MUI_PAGE_LICENSE "resources\eula.txt" # Adds a EULA page to the installer
!insertmacro MUI_PAGE_DIRECTORY # In which folder install page.
!insertmacro MUI_PAGE_INSTFILES # Installing page.
!insertmacro MUI_PAGE_FINISH # Finished installation page.

!insertmacro MUI_UNPAGE_CONFIRM # Ask before removing anything.
!insertmacro MUI_UNPAGE_INSTFILES # Uinstalling page

!insertmacro MUI_LANGUAGE "English" # Set the Language of the installer

## The following two statements can be used to sign the installer and the uninstaller. The path to the binaries are provided in %1
#!uninstfinalize 'signtool --file "%1"'
#!finalize 'signtool --file "%1"'

Name "${INFO_PRODUCTNAME}"
OutFile "..\..\bin\${INFO_PROJECTNAME}-${ARCH}-installer.exe" # Name of the installer's file.
InstallDir "$PROGRAMFILES64\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}" # Default installing folder ($PROGRAMFILES is Program Files folder).
ShowInstDetails show # This will always show the installation details.
ShowUninstDetails show
## The details list is the one part of the body NSIS will colour.
InstallColors E8E8EA 111112       ## --text on --bg-sidebar

####
## StopRunningOpenSave asks a running OpenSave to shut down, and waits.
##
## Without this the installer replaces files that are open: the running app
## holds its own executable and its SQLite database, so the copy fails and
## leaves a half-installed folder, or the uninstaller leaves the exe behind
## and the Start menu entry points at nothing.
##
## Asked, not killed. This app writes a database and zips save archives, and
## a snapshot interrupted halfway is not a snapshot — so it is sent the same
## shutdown the tray's Quit performs (see sysintegration.QuitFlag), which
## stops syncing, waits for archives in flight and closes the database.
## taskkill is the fallback for a build too old to know the flag, or an app
## already wedged, and only after the polite request has had its time.
####
!macro StopRunningOpenSave un
Function ${un}StopRunningOpenSave
    Push $0
    Push $1

    ; Nothing running? Nothing to do. FindProcDLL isn't available, so ask
    ; tasklist, which ships with Windows.
    nsExec::ExecToStack 'cmd /c tasklist /FI "IMAGENAME eq ${PRODUCT_EXECUTABLE}" /NH | find /I "${PRODUCT_EXECUTABLE}"'
    Pop $0
    Pop $1
    ${If} $0 != 0
        Goto done
    ${EndIf}

    DetailPrint "Closing ${INFO_PRODUCTNAME}..."
    ${If} ${FileExists} "$INSTDIR\${PRODUCT_EXECUTABLE}"
        nsExec::ExecToLog '"$INSTDIR\${PRODUCT_EXECUTABLE}" --quit'
        Pop $0
    ${EndIf}

    ; Up to ten seconds for a clean exit, checked every half second, so a
    ; quick shutdown costs half a second rather than the whole allowance.
    StrCpy $1 0
    ${Do}
        Sleep 500
        nsExec::ExecToStack 'cmd /c tasklist /FI "IMAGENAME eq ${PRODUCT_EXECUTABLE}" /NH | find /I "${PRODUCT_EXECUTABLE}"'
        Pop $0
        Pop $1
        ${If} $0 != 0
            DetailPrint "${INFO_PRODUCTNAME} closed."
            Goto done
        ${EndIf}
        IntOp $1 $1 + 1
        ${If} $1 >= 20
            ${Break}
        ${EndIf}
    ${Loop}

    DetailPrint "${INFO_PRODUCTNAME} did not close on request; stopping it."
    nsExec::ExecToLog 'taskkill /F /IM "${PRODUCT_EXECUTABLE}" /T'
    Pop $0
    Sleep 1000

done:
    Pop $1
    Pop $0
FunctionEnd
!macroend
!insertmacro StopRunningOpenSave ""
!insertmacro StopRunningOpenSave "un."

####
## Already installed? Say so, and offer the two things a person opening an
## installer for an app they already have actually wants: install it again
## (repair, or upgrade to this version) or remove it. Without this the
## installer silently reinstalls over the top, which is the right guess often
## enough to be confusing the rest of the time — someone who downloaded it to
## get rid of the app has no way in at all, and the uninstaller lives in a
## folder they would have to go looking for.
####
Function .onInit
    !insertmacro wails.checkArchitecture

    ReadRegStr $R0 ${UNINST_ROOT} "${UNINST_KEY}" "UninstallString"
    ReadRegStr $R1 ${UNINST_ROOT} "${UNINST_KEY}" "DisplayVersion"
    ${If} $R0 == ""
        Return ; not installed; carry on with a normal install
    ${EndIf}

    ReadRegStr $R2 ${UNINST_ROOT} "${UNINST_KEY}" "InstallLocation"
    ${If} $R2 != ""
        StrCpy $INSTDIR $R2
    ${EndIf}

    MessageBox MB_YESNOCANCEL|MB_ICONQUESTION|MB_DEFBUTTON1 \
        "${INFO_PRODUCTNAME} $R1 is already installed on this computer.$\r$\n$\r$\n\
        Yes  -  Install version ${INFO_PRODUCTVERSION} over it$\r$\n\
        No  -  Remove ${INFO_PRODUCTNAME} from this computer$\r$\n\
        Cancel  -  Leave everything as it is$\r$\n$\r$\n\
        Your games, snapshots and paired devices are kept either way. Removing asks about them separately." \
        /SD IDYES IDYES proceed IDNO uninstall

    ; Cancel
    Abort

uninstall:
    ; Hand over to the installed uninstaller and let it own the conversation
    ; about the data and the command-line tool.
    ;
    ; Two details this needs to get right. UninstallString is stored quoted,
    ; and ExecWait quotes what it is given, so the quotes come off first or
    ; the command is malformed and nothing runs at all. And the uninstaller
    ; is copied to the temp directory before being run: _?= is what makes
    ; ExecWait actually wait (it suppresses the copy-and-relaunch an
    ; uninstaller normally does), but an uninstaller running from inside the
    ; folder it is deleting cannot delete its own file, and would leave the
    ; folder and a stale uninstall.exe behind. Run from elsewhere, pointed at
    ; the real directory, it removes all of it.
    StrCpy $R3 $R0
    StrCpy $R4 $R3 1
    ${If} $R4 == '"'
        StrLen $R5 $R3
        IntOp $R5 $R5 - 2
        StrCpy $R3 $R3 $R5 1
    ${EndIf}
    ${If} $R2 == ""
        ${GetParent} "$R3" $R2
    ${EndIf}
    CopyFiles /SILENT "$R3" "$TEMP\${INFO_PRODUCTNAME}-uninstall.exe"
    ExecWait '"$TEMP\${INFO_PRODUCTNAME}-uninstall.exe" _?=$R2'
    Delete "$TEMP\${INFO_PRODUCTNAME}-uninstall.exe"
    Abort

proceed:
FunctionEnd

Section
    !insertmacro wails.setShellContext

    ; Before anything is written: the app holds its own files open.
    Call StopRunningOpenSave

    !insertmacro wails.webview2runtime

    SetOutPath $INSTDIR

    !insertmacro wails.files

    CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    CreateShortCut "$DESKTOP\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"

    !insertmacro wails.associateFiles
    !insertmacro wails.associateCustomProtocols

    !insertmacro wails.writeUninstaller

    ; Where it went, so re-running this installer finds the existing copy
    ; instead of offering to put a second one somewhere else. wails writes the
    ; rest of the uninstall key, the size included.
    SetRegView 64
    WriteRegStr ${UNINST_ROOT} "${UNINST_KEY}" "InstallLocation" "$INSTDIR"
SectionEnd

####
## The finish page's two checkboxes.
##
## The autostart entry is written for the user who is installing, not for the
## machine: it is the same HKCU Run value the app's own setting writes, and
## the app adopts it on next launch so the two never disagree. Launched with
## --start-hidden for the same reason the app writes it that way — someone
## who asked for syncing at boot did not ask for a window at boot.
####
Function LaunchOpenSave
    ; Through Explorer, which runs as the signed-in user, so the app does not
    ; inherit this installer's elevation. An elevated OpenSave writes its
    ; database and snapshots as the administrator, and the person's own next
    ; launch — unelevated, from the Start menu — then cannot read them. The
    ; ShellExecAsUser plugin does this more directly but is not part of a
    ; stock NSIS, and this has to build on a plain runner.
    Exec '"$WINDIR\explorer.exe" "$INSTDIR\${PRODUCT_EXECUTABLE}"'
FunctionEnd

Function EnableAutostart
    WriteRegStr HKCU "${OPENSAVE_RUN_KEY}" "${INFO_PRODUCTNAME}" \
        '"$INSTDIR\${PRODUCT_EXECUTABLE}" --start-hidden'
FunctionEnd

####
## $LOCALAPPDATA and $APPDATA follow SetShellVarContext, and installing runs
## with it set to "all" so the shortcuts land for everyone — which turns
## $LOCALAPPDATA into ProgramData. Anything belonging to the person rather
## than the machine has to be read with the context put back to "current".
##
## That person is whoever ran the installer. Elevating with a different
## administrator account moves HKCU and $PROFILE to that account, so the
## per-user parts would be looked for in the wrong profile. The shortcuts
## wails writes have the same limitation; it is the price of a per-machine
## install and not something to half-fix here.
####

Section "uninstall"
    !insertmacro wails.setShellContext

    ; The app first, or its own executable cannot be deleted.
    Call un.StopRunningOpenSave

    RMDir /r "$AppData\${PRODUCT_EXECUTABLE}" # Remove the WebView2 DataPath

    RMDir /r $INSTDIR

    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"

    ; The start-with-Windows entry. Left behind, Windows tries to launch a
    ; program that is no longer there at every boot, and the person who
    ; uninstalled OpenSave has to find out what a stale Run entry is to make
    ; it stop.
    DeleteRegValue HKCU "${OPENSAVE_RUN_KEY}" "${INFO_PRODUCTNAME}"

    ; The command-line tool installs itself separately, outside this folder,
    ; and a `opensave` that still answers after the app is gone is a surprise
    ; — but it is also a thing people use on its own, so it is asked about
    ; rather than assumed. Its own uninstaller handles the PATH entry, which
    ; is not something to edit with string operations.
    SetShellVarContext current
    ${If} ${FileExists} "${OPENSAVE_CLI_DIR}\opensave.exe"
        MessageBox MB_YESNO|MB_ICONQUESTION \
            "Also remove the OpenSave command-line tool?$\r$\n$\r$\n\
            It was installed separately, in $LOCALAPPDATA, and works without the app." \
            /SD IDNO IDNO skipcli
        nsExec::ExecToLog '"${OPENSAVE_CLI_DIR}\opensave.exe" install --uninstall --yes'
        Pop $0
skipcli:
    ${EndIf}

    ; And finally the data, which is the one thing here that cannot be got
    ; back. Defaulted to keeping it, and to keeping it in silent mode: an
    ; uninstall is not consent to delete every snapshot of every save.
    ${If} ${FileExists} "${OPENSAVE_DATA_DIR}\*.*"
        MessageBox MB_YESNO|MB_ICONEXCLAMATION|MB_DEFBUTTON2 \
            "Delete your OpenSave data as well?$\r$\n$\r$\n\
            This is every snapshot of every save, your settings, and your paired devices, in$\r$\n\
            ${OPENSAVE_DATA_DIR}$\r$\n$\r$\n\
            Your games' own save files are NOT here and are never touched.$\r$\n$\r$\n\
            Choose No to keep it - reinstalling OpenSave picks it up again." \
            /SD IDNO IDNO keepdata
        RMDir /r "${OPENSAVE_DATA_DIR}"
        DetailPrint "Removed ${OPENSAVE_DATA_DIR}"
        Goto datadone
keepdata:
        DetailPrint "Kept your snapshots and settings in ${OPENSAVE_DATA_DIR}"
datadone:
    ${EndIf}

    !insertmacro wails.unassociateFiles
    !insertmacro wails.unassociateCustomProtocols

    !insertmacro wails.deleteUninstaller
SectionEnd
