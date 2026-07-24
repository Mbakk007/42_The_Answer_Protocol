package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// ── client state ──────────────────────────────────────────────────────────────

type Client struct {
	conn net.Conn
	mu   sync.Mutex
	win  fyne.Window

	// room panel
	roomTitle   *widget.Label
	roomDesc    *widget.Label
	exitButtons *fyne.Container
	itemsList   *widget.List
	npcsList    *widget.List
	playersList *widget.List

	// chat / log
	globalLog *widget.List
	roomLog   *widget.List
	groupLog  *widget.List
	logView   *widget.List

	// inventory
	inventoryList *widget.List

	// status bar
	statusHP      *widget.Label
	statusRoom    *widget.Label
	statusPlayers *widget.Label
	statusCombat  *widget.Label

	// command input
	cmdInput *widget.Entry

	// data (guarded by mu)
	exits       []string
	items       []string
	npcs        []string
	roomPlayers []string
	inventory   []string
	globalMsgs  []string
	roomMsgs    []string
	groupMsgs   []string
	logMsgs     []string
}

// ── main ──────────────────────────────────────────────────────────────────────

func main() {
	addr := "localhost:5050"
	if len(os.Args) == 2 {
		addr = os.Args[1]
	}

	a := app.New()
	w := a.NewWindow("TAP — The Answer Protocol")
	w.Resize(fyne.NewSize(1100, 700))

	c := &Client{win: w}
	c.buildWidgets()

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		dialog.ShowError(fmt.Errorf("cannot connect to %s: %w", addr, err), w)
	} else {
		c.conn = conn
		c.addLog("connected to " + addr)
		go c.readServer()
	}

	w.SetContent(c.buildLayout())
	w.ShowAndRun()
}

// ── widget construction ───────────────────────────────────────────────────────

func (c *Client) buildWidgets() {
	c.roomTitle = widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	c.roomDesc = widget.NewLabel("")
	c.roomDesc.Wrapping = fyne.TextWrapWord
	c.exitButtons = container.NewHBox()

	c.itemsList = widget.NewList(
		func() int { c.mu.Lock(); defer c.mu.Unlock(); return len(c.items) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			c.mu.Lock(); defer c.mu.Unlock()
			if i < len(c.items) {
				o.(*widget.Label).SetText("📦 " + c.items[i])
			}
		},
	)
	c.itemsList.OnSelected = func(id widget.ListItemID) {
		c.mu.Lock()
		if id < len(c.items) {
			item := c.items[id]
			c.mu.Unlock()
			c.send("TAKE " + item)
		} else {
			c.mu.Unlock()
		}
		c.itemsList.UnselectAll()
	}

	c.npcsList = widget.NewList(
		func() int { c.mu.Lock(); defer c.mu.Unlock(); return len(c.npcs) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			c.mu.Lock(); defer c.mu.Unlock()
			if i < len(c.npcs) {
				o.(*widget.Label).SetText("🧍 " + c.npcs[i])
			}
		},
	)
	c.npcsList.OnSelected = func(id widget.ListItemID) {
		c.mu.Lock()
		if id < len(c.npcs) {
			npc := c.npcs[id]
			c.mu.Unlock()
			c.showNPCDialog(npc)
		} else {
			c.mu.Unlock()
		}
		c.npcsList.UnselectAll()
	}

	c.playersList = widget.NewList(
		func() int { c.mu.Lock(); defer c.mu.Unlock(); return len(c.roomPlayers) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			c.mu.Lock(); defer c.mu.Unlock()
			if i < len(c.roomPlayers) {
				o.(*widget.Label).SetText("👤 " + c.roomPlayers[i])
			}
		},
	)

	c.globalLog = c.newMsgList(&c.globalMsgs)
	c.roomLog = c.newMsgList(&c.roomMsgs)
	c.groupLog = c.newMsgList(&c.groupMsgs)
	c.logView = c.newMsgList(&c.logMsgs)

	c.inventoryList = widget.NewList(
		func() int { c.mu.Lock(); defer c.mu.Unlock(); return len(c.inventory) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			c.mu.Lock(); defer c.mu.Unlock()
			if i < len(c.inventory) {
				o.(*widget.Label).SetText("🎒 " + c.inventory[i])
			}
		},
	)
	c.inventoryList.OnSelected = func(id widget.ListItemID) {
		c.mu.Lock()
		if id < len(c.inventory) {
			item := c.inventory[id]
			c.mu.Unlock()
			c.send("DROP " + item)
		} else {
			c.mu.Unlock()
		}
		c.inventoryList.UnselectAll()
	}

	c.statusHP = widget.NewLabel("HP: --")
	c.statusRoom = widget.NewLabel("Room: --")
	c.statusPlayers = widget.NewLabel("Players: --")
	c.statusCombat = widget.NewLabel("")

	c.cmdInput = widget.NewEntry()
	c.cmdInput.SetPlaceHolder("type command and press Enter...")
	c.cmdInput.OnSubmitted = func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		c.send(s)
		c.cmdInput.SetText("")
	}
}

func (c *Client) newMsgList(msgs *[]string) *widget.List {
	return widget.NewList(
		func() int { c.mu.Lock(); defer c.mu.Unlock(); return len(*msgs) },
		func() fyne.CanvasObject {
			l := widget.NewLabel("")
			l.Wrapping = fyne.TextWrapWord
			return l
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			c.mu.Lock(); defer c.mu.Unlock()
			if i < len(*msgs) {
				o.(*widget.Label).SetText((*msgs)[i])
			}
		},
	)
}

// ── layout ────────────────────────────────────────────────────────────────────

func (c *Client) buildLayout() fyne.CanvasObject {
	// left: room info
	leftPanel := container.NewVBox(
		c.roomTitle,
		c.roomDesc,
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Exits", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		c.exitButtons,
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Items (click to TAKE)", fyne.TextAlignLeading, fyne.TextStyle{Italic: true}),
		c.itemsList,
		widget.NewSeparator(),
		widget.NewLabelWithStyle("NPCs (click to interact)", fyne.TextAlignLeading, fyne.TextStyle{Italic: true}),
		c.npcsList,
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Players here", fyne.TextAlignLeading, fyne.TextStyle{Italic: true}),
		c.playersList,
	)
	leftScroll := container.NewScroll(leftPanel)
	leftScroll.SetMinSize(fyne.NewSize(260, 0))

	// centre: chat tabs
	chatTabs := container.NewAppTabs(
		container.NewTabItem("Global", c.globalLog),
		container.NewTabItem("Room", c.roomLog),
		container.NewTabItem("Group", c.groupLog),
		container.NewTabItem("Log", c.logView),
	)
	chatInput := widget.NewEntry()
	chatInput.SetPlaceHolder("message...")
	scopeSelect := widget.NewSelect([]string{"GLOBAL", "ROOM", "GROUP"}, nil)
	scopeSelect.SetSelected("GLOBAL")
	sendBtn := widget.NewButtonWithIcon("Send", theme.MailSendIcon(), func() {
		msg := strings.TrimSpace(chatInput.Text)
		if msg == "" {
			return
		}
		c.send(fmt.Sprintf("CHAT %s %s", scopeSelect.Selected, msg))
		chatInput.SetText("")
	})
	chatRow := container.NewBorder(nil, nil, scopeSelect, sendBtn, chatInput)
	centre := container.NewBorder(nil, chatRow, nil, nil, chatTabs)

	// right: inventory + actions
	inventorySection := container.NewVBox(
		widget.NewLabelWithStyle("Inventory (click to DROP)", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		c.inventoryList,
	)

	actionButtons := container.NewGridWithColumns(2,
		widget.NewButtonWithIcon("LOOK", theme.SearchIcon(), func() { c.send("LOOK") }),
		widget.NewButtonWithIcon("STATUS", theme.InfoIcon(), func() { c.send("STATUS") }),
		widget.NewButtonWithIcon("INVENTORY", theme.ListIcon(), func() { c.send("INVENTORY") }),
		widget.NewButtonWithIcon("WHO", theme.AccountIcon(), func() { c.send("WHO") }),
		widget.NewButtonWithIcon("QUESTS", theme.DocumentIcon(), func() { c.send("QUESTS") }),
		widget.NewButtonWithIcon("FLEE", theme.NavigateBackIcon(), func() { c.send("FLEE") }),
		widget.NewButtonWithIcon("QUIT", theme.LogoutIcon(), func() {
			c.send("QUIT")
			c.win.Close()
		}),
	)

	moveButtons := container.NewGridWithColumns(3,
		layout.NewSpacer(),
		widget.NewButton("↑ N", func() { c.send("MOVE north") }),
		layout.NewSpacer(),
		widget.NewButton("← W", func() { c.send("MOVE west") }),
		widget.NewButton("· ·", nil),
		widget.NewButton("E →", func() { c.send("MOVE east") }),
		layout.NewSpacer(),
		widget.NewButton("↓ S", func() { c.send("MOVE south") }),
		layout.NewSpacer(),
	)

	groupInput := widget.NewEntry()
	groupInput.SetPlaceHolder("group name or player...")
	groupButtons := container.NewGridWithColumns(2,
		widget.NewButton("Create", func() {
			if groupInput.Text != "" {
				c.send("GROUP CREATE " + groupInput.Text)
			}
		}),
		widget.NewButton("Join", func() {
			if groupInput.Text != "" {
				c.send("GROUP JOIN " + groupInput.Text)
			}
		}),
		widget.NewButton("Invite", func() {
			if groupInput.Text != "" {
				c.send("GROUP INVITE " + groupInput.Text)
			}
		}),
		widget.NewButton("Leave", func() { c.send("GROUP LEAVE") }),
	)

	rightPanel := container.NewVBox(
		inventorySection,
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Actions", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		actionButtons,
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Move", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		moveButtons,
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Group", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		groupInput,
		groupButtons,
	)
	rightScroll := container.NewScroll(rightPanel)
	rightScroll.SetMinSize(fyne.NewSize(220, 0))

	// status bar
	statusBar := container.NewHBox(
		c.statusHP,
		widget.NewSeparator(),
		c.statusRoom,
		widget.NewSeparator(),
		c.statusPlayers,
		widget.NewSeparator(),
		c.statusCombat,
	)

	// connect row at top
	nameEntry := widget.NewEntry()
	nameEntry.SetPlaceHolder("player name")
	connectBtn := widget.NewButton("Connect", func() {
		name := strings.TrimSpace(nameEntry.Text)
		if name == "" {
			return
		}
		c.send("CONNECT " + name)
	})
	connectRow := container.NewBorder(nil, nil, widget.NewLabel("Name:"), connectBtn, nameEntry)

	cmdRow := container.NewBorder(nil, nil, widget.NewLabel(">"), nil, c.cmdInput)

	return container.NewBorder(
		container.NewVBox(connectRow, widget.NewSeparator()),
		container.NewVBox(widget.NewSeparator(), statusBar, cmdRow),
		leftScroll,
		rightScroll,
		centre,
	)
}

// ── server reader ─────────────────────────────────────────────────────────────

func (c *Client) send(cmd string) {
	if c.conn == nil {
		c.addLog("not connected to server")
		return
	}
	fmt.Fprintf(c.conn, "%s\n", cmd)
	c.addLog("> " + cmd)
}

func (c *Client) readServer() {
	scanner := bufio.NewScanner(c.conn)
	for scanner.Scan() {
		c.handleLine(scanner.Text())
	}
	c.addLog("--- server disconnected ---")
}

func (c *Client) handleLine(line string) {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return
	}

	switch parts[0] {
	case "OK":
		rest := strings.TrimPrefix(line, "OK ")
		c.addLog("← " + line)
		c.handleOK(rest, parts)

	case "ERR":
		c.addLog("⚠ " + line)

	case "HINT":
		c.addLog("💡 " + strings.TrimPrefix(line, "HINT "))

	case "EVT":
		c.handleEVT(parts, line)

	default:
		c.addLog("← " + line)
	}
}

func (c *Client) handleOK(rest string, parts []string) {
	switch {
	case strings.HasPrefix(rest, "hello proto="):
		c.addLog("server ready — enter your name above and click Connect")

	case strings.HasPrefix(rest, "connected"):
		c.addLog("✅ connected to game world")
		c.send("LOOK")
		c.send("STATUS")
		c.send("INVENTORY")

	case strings.HasPrefix(rest, "room="):
		c.send("LOOK")

	case strings.HasPrefix(rest, "taken="):
		item := strings.TrimPrefix(rest, "taken=")
		c.mu.Lock()
		c.inventory = append(c.inventory, item)
		c.mu.Unlock()
		c.inventoryList.Refresh()
		c.send("LOOK")

	case strings.HasPrefix(rest, "dropped="):
		item := strings.TrimPrefix(rest, "dropped=")
		c.mu.Lock()
		for i, v := range c.inventory {
			if v == item {
				c.inventory = append(c.inventory[:i], c.inventory[i+1:]...)
				break
			}
		}
		c.mu.Unlock()
		c.inventoryList.Refresh()
		c.send("LOOK")

	case rest == "bye":
		c.addLog("disconnected")

	case strings.HasPrefix(rest, "{"):
		c.parseJSON(rest)

	case strings.HasPrefix(rest, "["):
		c.parseJSONArray(rest)
	}
}

func (c *Client) handleEVT(parts []string, line string) {
	if len(parts) < 3 {
		return
	}
	category := parts[1]
	evtType := parts[2]

	switch category {
	case "GLOBAL":
		if evtType == "CHAT" && len(parts) >= 4 {
			msg := fmt.Sprintf("[%s] %s", parts[3], strings.Join(parts[4:], " "))
			c.addGlobal(msg)
		}

	case "ROOM":
		switch evtType {
		case "PRESENCE":
			if len(parts) >= 5 {
				c.addRoom(fmt.Sprintf("*** %s %sed", parts[4], strings.ToLower(parts[3])))
				c.send("LOOK")
			}
		case "CHAT":
			if len(parts) >= 4 {
				c.addRoom(fmt.Sprintf("[%s] %s", parts[3], strings.Join(parts[4:], " ")))
			}
		case "ITEM":
			c.send("LOOK")
		}

	case "GROUP":
		switch evtType {
		case "CHAT":
			if len(parts) >= 4 {
				c.addGroup(fmt.Sprintf("[%s] %s", parts[3], strings.Join(parts[4:], " ")))
			}
		case "INVITE":
			if len(parts) >= 5 {
				c.addGroup(fmt.Sprintf("*** %s invited you to group %s", parts[3], parts[4]))
			}
		case "JOIN":
			if len(parts) >= 4 {
				c.addGroup(fmt.Sprintf("*** %s joined", parts[3]))
			}
		case "LEAVE":
			if len(parts) >= 4 {
				c.addGroup(fmt.Sprintf("*** %s left", parts[3]))
			}
		}

	case "COMBAT":
		switch evtType {
		case "ATTACK":
			if len(parts) >= 6 {
				c.addLog(fmt.Sprintf("⚔ %s attacks %s for %s", parts[3], parts[4], parts[5]))
			}
		case "DEFEAT":
			c.addLog(fmt.Sprintf("💀 %s defeated %s!", parts[3], parts[4]))
			c.send("LOOK")
			c.send("STATUS")
		case "DEATH":
			c.addLog(fmt.Sprintf("💀 %s was defeated by %s", parts[3], parts[4]))
			c.send("LOOK")
			c.send("STATUS")
		}

	case "QUEST":
		if evtType == "COMPLETE" && len(parts) >= 4 {
			c.addGlobal(fmt.Sprintf("🏆 %s completed: %s", parts[3], strings.Join(parts[4:], " ")))
		}

	case "STATS":
		val := strings.TrimPrefix(line, "EVT STATS ")
		c.statusPlayers.SetText("Server: " + val)
	}
}

// ── JSON parsing ──────────────────────────────────────────────────────────────

func (c *Client) parseJSON(s string) {
	switch {
	case strings.Contains(s, `"room"`):
		// LOOK response
		roomName := jsonStr(s, "name")
		roomDesc := jsonStr(s, "description")
		roomID := jsonStr(s, "id")

		exitsRaw := jsonObj(s, "exits")
		var exitDirs []string
		for _, dir := range []string{"north", "south", "east", "west", "up", "down"} {
			if strings.Contains(exitsRaw, `"`+dir+`"`) {
				exitDirs = append(exitDirs, dir)
			}
		}

		items := jsonArray(s, "items")
		npcIDs := jsonArray(s, "npcs")
		players := jsonArray(s, "players")

		c.mu.Lock()
		c.exits = exitDirs
		c.items = items
		c.npcs = npcIDs
		c.roomPlayers = players
		c.mu.Unlock()

		c.roomTitle.SetText(roomName)
		c.roomDesc.SetText(roomDesc)
		c.statusRoom.SetText("Room: " + roomID)

		c.exitButtons.Objects = nil
		for _, dir := range exitDirs {
			d := dir
			label := strings.ToUpper(d[:1]) + d[1:]
			c.exitButtons.Objects = append(c.exitButtons.Objects,
				widget.NewButton(label, func() { c.send("MOVE " + d) }),
			)
		}
		c.exitButtons.Refresh()
		c.itemsList.Refresh()
		c.npcsList.Refresh()
		c.playersList.Refresh()

	case strings.Contains(s, `"max_hp"`):
		// STATUS response
		hp := jsonNum(s, "hp")
		maxHP := jsonNum(s, "max_hp")
		status := jsonStr(s, "status")
		c.statusHP.SetText(fmt.Sprintf("HP: %s/%s", hp, maxHP))
		c.statusCombat.SetText(status)

	case strings.Contains(s, `"attacker_hp"`):
		// ATTACK response
		attackerHP := jsonNum(s, "attacker_hp")
		targetHP := jsonNum(s, "target_hp")
		damage := jsonNum(s, "damage")
		status := jsonStr(s, "status")
		c.addLog(fmt.Sprintf("⚔ dealt %s dmg | target hp=%s | your hp=%s | %s", damage, targetHP, attackerHP, status))
		c.statusHP.SetText("HP: " + attackerHP)
		c.statusCombat.SetText(status)
		c.send("LOOK")

	case strings.Contains(s, `"quest_id"`):
		// QUEST response
		questID := jsonStr(s, "quest_id")
		desc := jsonStr(s, "description")
		reward := jsonStr(s, "reward")
		status := jsonStr(s, "status")
		c.addLog(fmt.Sprintf("📜 [%s] %s — %s (reward: %s)", status, questID, desc, reward))
	}
}

func (c *Client) parseJSONArray(s string) {
	if strings.Contains(s, `"quest_id"`) {
		c.addLog("📜 Quest log: " + s)
		return
	}
	// inventory
	items := jsonArray(s, "")
	c.mu.Lock()
	c.inventory = items
	c.mu.Unlock()
	c.inventoryList.Refresh()
}

// ── message helpers ───────────────────────────────────────────────────────────

func (c *Client) addLog(msg string) {
	c.mu.Lock()
	c.logMsgs = append(c.logMsgs, msg)
	n := len(c.logMsgs)
	c.mu.Unlock()
	c.logView.Refresh()
	c.logView.ScrollTo(n - 1)
}

func (c *Client) addGlobal(msg string) {
	c.mu.Lock()
	c.globalMsgs = append(c.globalMsgs, msg)
	n := len(c.globalMsgs)
	c.mu.Unlock()
	c.globalLog.Refresh()
	c.globalLog.ScrollTo(n - 1)
	c.addLog(msg)
}

func (c *Client) addRoom(msg string) {
	c.mu.Lock()
	c.roomMsgs = append(c.roomMsgs, msg)
	n := len(c.roomMsgs)
	c.mu.Unlock()
	c.roomLog.Refresh()
	c.roomLog.ScrollTo(n - 1)
	c.addLog(msg)
}

func (c *Client) addGroup(msg string) {
	c.mu.Lock()
	c.groupMsgs = append(c.groupMsgs, msg)
	n := len(c.groupMsgs)
	c.mu.Unlock()
	c.groupLog.Refresh()
	c.groupLog.ScrollTo(n - 1)
	c.addLog(msg)
}

// ── NPC dialog ────────────────────────────────────────────────────────────────

func (c *Client) showNPCDialog(npcID string) {
	talkBtn := widget.NewButton("TALK", func() { c.send("TALK " + npcID) })
	attackBtn := widget.NewButton("ATTACK", func() { c.send("ATTACK " + npcID) })
	questBtn := widget.NewButton("QUEST", func() { c.send("QUEST " + npcID) })
	content := container.NewVBox(
		widget.NewLabel("What do you want to do with: "+npcID+"?"),
		container.NewHBox(talkBtn, attackBtn, questBtn),
	)
	dialog.NewCustom("NPC: "+npcID, "Close", content, c.win).Show()
}

// ── minimal JSON helpers ──────────────────────────────────────────────────────

func jsonStr(s, key string) string {
	needle := `"` + key + `":"`
	idx := strings.Index(s, needle)
	if idx < 0 {
		return ""
	}
	rest := s[idx+len(needle):]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return rest
	}
	return rest[:end]
}

func jsonNum(s, key string) string {
	needle := `"` + key + `":`
	idx := strings.Index(s, needle)
	if idx < 0 {
		return "0"
	}
	rest := s[idx+len(needle):]
	end := strings.IndexAny(rest, ",}")
	if end < 0 {
		return strings.TrimSpace(rest)
	}
	return strings.TrimSpace(rest[:end])
}

func jsonObj(s, key string) string {
	needle := `"` + key + `":{`
	idx := strings.Index(s, needle)
	if idx < 0 {
		return ""
	}
	start := idx + len(needle) - 1
	depth := 0
	for i := start; i < len(s); i++ {
		if s[i] == '{' {
			depth++
		} else if s[i] == '}' {
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}

func jsonArray(s, key string) []string {
	var raw string
	if key == "" {
		raw = s
	} else {
		needle := `"` + key + `":[`
		idx := strings.Index(s, needle)
		if idx < 0 {
			return nil
		}
		raw = s[idx+len(needle)-1:]
	}
	start := strings.Index(raw, "[")
	if start < 0 {
		return nil
	}
	end := strings.Index(raw[start:], "]")
	if end < 0 {
		return nil
	}
	inner := strings.TrimSpace(raw[start+1 : start+end])
	if inner == "" {
		return nil
	}
	var result []string
	for _, part := range strings.Split(inner, ",") {
		part = strings.TrimSpace(strings.Trim(part, `"`))
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}