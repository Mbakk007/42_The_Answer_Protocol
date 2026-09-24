package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"tap/world"
)

type lookReply struct {
	Room    world.Location `json:"room"`
	Players []string       `json:"players"`
	Items   []string       `json:"items"`
	NPCs    []string       `json:"npcs"`
}

func main() {
	addr := "localhost:4040"
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	send := func(format string, args ...any) {
		fmt.Fprintf(conn, format+"\n", args...)
	}

	pending := 0
	refresh := func() {
		pending += 3
		send("LOOK")
		send("INVENTORY")
		send("WHO")
	}
	auto := func() bool {
		if pending > 0 {
			pending--
			return true
		}
		return false
	}

	a := app.New()
	w := a.NewWindow("TAP")

	global := widget.NewMultiLineEntry()
	room := widget.NewMultiLineEntry()
	group := widget.NewMultiLineEntry()
	logv := widget.NewMultiLineEntry()
	for _, b := range []*widget.Entry{global, room, group, logv} {
		b.Disable()
	}

	tabs := container.NewAppTabs(
		container.NewTabItem("Global", global),
		container.NewTabItem("Room", room),
		container.NewTabItem("Group", group),
		container.NewTabItem("Log", logv),
	)

	roomName := widget.NewLabel("")
	roomDesc := widget.NewLabel("")
	roomDesc.Wrapping = fyne.TextWrapWord
	dialogue := widget.NewLabel("")
	dialogue.Wrapping = fyne.TextWrapWord
	counters := widget.NewLabel("players here: 0 | online: 0")

	exitBox := container.NewHBox()
	itemBox := container.NewVBox()
	npcBox := container.NewVBox()
	invBox := container.NewVBox()

	hereCount, onlineCount := 0, 0
	setCounters := func() {
		counters.SetText(fmt.Sprintf("players here: %d | online: %d", hereCount, onlineCount))
	}

	entry := widget.NewEntry()
	entry.SetPlaceHolder("type a raw protocol command, e.g. CONNECT alice")
	entry.OnSubmitted = func(string) {
		send("%s", entry.Text)
		entry.SetText("")
	}

	appendTo := func(box *widget.Entry, line string) {
		fyne.Do(func() {
			box.SetText(box.Text + line + "\n")
			box.CursorRow = strings.Count(box.Text, "\n")
		})
	}

	go func() {
		sc := bufio.NewScanner(conn)
		for sc.Scan() {
			line := sc.Text()
			switch {
			case strings.HasPrefix(line, "EVT GLOBAL CHAT"):
				appendTo(global, line)
			case strings.HasPrefix(line, "EVT ROOM CHAT"):
				appendTo(room, line)
			case strings.HasPrefix(line, "EVT GROUP CHAT"):
				appendTo(group, line)

			case strings.HasPrefix(line, "EVT ROOM PRESENCE"):
				appendTo(logv, line)
				refresh()

			case strings.HasPrefix(line, `OK {"room"`):
				if !auto() {
					appendTo(logv, line)
				}
				var lr lookReply
				if err := json.Unmarshal([]byte(line[3:]), &lr); err != nil {
					appendTo(logv, line)
					break
				}
				fyne.Do(func() {
					roomName.SetText(lr.Room.Name)
					roomDesc.SetText(lr.Room.Description)
					hereCount = len(lr.Players)
					setCounters()

					exitBox.RemoveAll()
					for d := range lr.Room.Exits {
						dir := d
						exitBox.Add(widget.NewButton(dir, func() {
							send("MOVE %s", dir)
						}))
					}

					itemBox.RemoveAll()
					for _, it := range lr.Items {
						id := it
						itemBox.Add(widget.NewButton("take "+id, func() {
							send("TAKE %s", id)
							refresh()
						}))
					}

					npcBox.RemoveAll()
					for _, n := range lr.NPCs {
						id := n
						npcBox.Add(container.NewHBox(
							widget.NewLabel(id),
							widget.NewButton("talk", func() { send("TALK %s", id) }),
							widget.NewButton("quest", func() { send("QUEST %s", id) }),
							widget.NewButton("attack", func() {
								send("ATTACK %s", id)
								refresh()
							}),
						))
					}
				})

			case strings.HasPrefix(line, "OK ["):
				var inv []string
				if err := json.Unmarshal([]byte(line[3:]), &inv); err != nil {
					appendTo(logv, line)
					break
				}
				if !auto() {
					appendTo(logv, line)
				}
				fyne.Do(func() {
					invBox.RemoveAll()
					for _, it := range inv {
						id := it
						invBox.Add(widget.NewButton("drop "+id, func() {
							send("DROP %s", id)
							refresh()
						}))
					}
				})

			case strings.HasPrefix(line, "OK players="):
				if !auto() {
					appendTo(logv, line)
				}
				n := 0
				fmt.Sscanf(line, "OK players=%d", &n)
				fyne.Do(func() {
					onlineCount = n
					setCounters()
				})

			case strings.HasPrefix(line, "OK ") && !strings.HasPrefix(line, "OK {"):
				appendTo(logv, line)
				fyne.Do(func() { dialogue.SetText(line[3:]) })

			default:
				appendTo(logv, line)
			}
		}
		if err := sc.Err(); err != nil {
			log.Printf("read error: %v", err)
		}
	}()

	cmdButton := func(name string) *widget.Button {
		return widget.NewButton(name, func() { send("%s", name) })
	}
	actions := container.NewHBox(
		cmdButton("LOOK"),
		cmdButton("WHO"),
		cmdButton("STATUS"),
		cmdButton("INVENTORY"),
		cmdButton("QUESTS"),
		widget.NewButton("GROUP CREATE", func() { send("GROUP CREATE") }),
		widget.NewButton("GROUP LEAVE", func() { send("GROUP LEAVE") }),
		cmdButton("QUIT"),
	)

	nameEntry := widget.NewEntry()
	nameEntry.SetPlaceHolder("name")
	connectBtn := widget.NewButton("CONNECT", func() {
		send("CONNECT %s", nameEntry.Text)
		refresh()
	})

	chatEntry := widget.NewEntry()
	chatEntry.SetPlaceHolder("message")
	scope := widget.NewSelect([]string{"GLOBAL", "ROOM", "GROUP"}, nil)
	scope.SetSelected("GLOBAL")
	chatBtn := widget.NewButton("CHAT", func() {
		if chatEntry.Text == "" {
			return
		}
		send("CHAT %s %s", scope.Selected, chatEntry.Text)
		chatEntry.SetText("")
	})

	topControls := container.NewVBox(
		container.NewBorder(nil, nil, nil, connectBtn, nameEntry),
		actions,
	)

	chatBar := container.NewBorder(nil, nil, scope, chatBtn, chatEntry)
	bottom := container.NewVBox(chatBar, entry)

	roomPanel := container.NewVBox(
		roomName,
		roomDesc,
		widget.NewLabel("exits:"), exitBox,
		widget.NewLabel("items:"), itemBox,
		widget.NewLabel("npcs:"), npcBox,
		widget.NewLabel("inventory:"), invBox,
		widget.NewLabel("dialogue:"), dialogue,
		counters,
	)

	w.SetContent(container.NewBorder(
		topControls,
		bottom,
		container.NewVScroll(roomPanel),
		nil,
		tabs,
	))
	w.Resize(fyne.NewSize(1000, 620))
	w.ShowAndRun()
}
