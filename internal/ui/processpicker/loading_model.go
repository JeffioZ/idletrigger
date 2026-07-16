package processpicker

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/JeffioZ/idletrigger/internal/automation"
	"github.com/JeffioZ/idletrigger/internal/platform/windows/processcatalog"
)

func (p *picker) startLoad(mode processLoadMode) {
	if p.closed.Load() || p.hwnd == 0 {
		return
	}
	p.captureSelection()
	p.captureViewAnchors()
	p.loading = true
	p.enriching = false
	p.setText(idStatus, p.text("process_picker_loading"))
	p.setText(idEmpty, p.text("process_picker_loading"))
	p.show(idEmpty, len(p.visible) == 0)
	p.enable(idRefresh, false)
	p.enable(idConfirm, len(p.selected) > 0)
	generation := nextGeneration.Add(1)
	p.generation = generation
	var exactTargets []automation.ProcessTarget
	for _, target := range p.selected {
		if target.Match == automation.MatchPath {
			exactTargets = append(exactTargets, target)
		}
	}
	cachedDescriptions := make(map[string]string, len(p.descriptions))
	for key, value := range p.descriptions {
		cachedDescriptions[key] = value
	}
	attemptedDescriptions := make(map[string]struct{}, len(p.descriptionTried))
	for key := range p.descriptionTried {
		attemptedDescriptions[key] = struct{}{}
	}
	p.workers.Add(1)
	go func() {
		defer p.workers.Done()
		scanStarted := time.Now()
		instances, err := snapshotNamesForPicker()
		scanDuration := time.Since(scanStarted)
		if err != nil {
			postPickerUI(func() { p.finishLoad(generation, nil, err, true, scanDuration) })
			return
		}
		for index := range instances {
			key := (automation.ProcessTarget{Match: automation.MatchName, Executable: instances[index].Executable}).Key()
			instances[index].Description = cachedDescriptions[key]
		}
		candidates, nameAttempts := descriptionCandidates(instances, cachedDescriptions, attemptedDescriptions, mode == processLoadManual)
		exactAttempts := make([]automation.ProcessTarget, 0, len(exactTargets))
		for _, target := range exactTargets {
			key := target.Key()
			if cachedDescriptions[key] != "" {
				continue
			}
			if _, tried := attemptedDescriptions[key]; tried && mode != processLoadManual {
				continue
			}
			exactAttempts = append(exactAttempts, target)
		}
		hasMetadataWork := len(nameAttempts) > 0 || len(exactAttempts) > 0
		postPickerUI(func() { p.finishLoad(generation, instances, nil, !hasMetadataWork, scanDuration) })
		nameDescriptions := make(map[string]string, len(nameAttempts))
		if len(candidates) > 0 {
			for _, instance := range enrichDescriptionsForPicker(candidates) {
				if instance.Description == "" {
					continue
				}
				key := (automation.ProcessTarget{Match: automation.MatchName, Executable: instance.Executable}).Key()
				nameDescriptions[key] = instance.Description
			}
		}
		exactDescriptions := make(map[string]string, len(exactAttempts))
		for _, target := range exactAttempts {
			if description := fileDescriptionForPicker(target.Path); description != "" {
				exactDescriptions[target.Key()] = description
			}
		}
		if hasMetadataWork {
			postPickerUI(func() {
				p.finishDescriptionLoad(generation, instances, nameAttempts, nameDescriptions, exactAttempts, exactDescriptions)
			})
		}
	}()
}

func descriptionCandidates(instances []processcatalog.Instance, cached map[string]string, attempted map[string]struct{}, retry bool) ([]processcatalog.Instance, []string) {
	wanted := make(map[string]struct{})
	attempts := make([]string, 0)
	for _, instance := range instances {
		key := (automation.ProcessTarget{Match: automation.MatchName, Executable: instance.Executable}).Key()
		if cached[key] != "" {
			continue
		}
		if _, tried := attempted[key]; tried && !retry {
			continue
		}
		if _, exists := wanted[key]; !exists {
			wanted[key] = struct{}{}
			attempts = append(attempts, key)
		}
	}
	candidates := make([]processcatalog.Instance, 0, len(instances))
	for _, instance := range instances {
		key := (automation.ProcessTarget{Match: automation.MatchName, Executable: instance.Executable}).Key()
		if _, ok := wanted[key]; ok {
			candidates = append(candidates, instance)
		}
	}
	return candidates, attempts
}

func (p *picker) finishDescriptionLoad(generation uint64, instances []processcatalog.Instance, nameAttempts []string, nameDescriptions map[string]string, exactAttempts []automation.ProcessTarget, exactDescriptions map[string]string) {
	if p.closed.Load() || p.hwnd == 0 || p.generation != generation {
		return
	}
	for _, key := range nameAttempts {
		p.descriptionTried[key] = struct{}{}
	}
	for _, target := range exactAttempts {
		p.descriptionTried[target.Key()] = struct{}{}
	}
	for key, value := range nameDescriptions {
		p.descriptions[key] = value
	}
	for key, value := range exactDescriptions {
		p.descriptions[key] = value
	}
	for index := range instances {
		key := (automation.ProcessTarget{Match: automation.MatchName, Executable: instances[index].Executable}).Key()
		instances[index].Description = p.descriptions[key]
	}
	p.finishLoad(generation, instances, nil, true, 0)
}

func (p *picker) finishLoad(generation uint64, instances []processcatalog.Instance, err error, final bool, scanDuration time.Duration) {
	if p.closed.Load() || p.hwnd == 0 || p.generation != generation {
		return
	}
	if err != nil {
		p.loading = false
		p.enriching = false
		p.setText(idStatus, fmt.Sprintf(p.text("process_picker_error"), err.Error()))
		p.setText(idEmpty, p.text("process_picker_load_failed"))
		p.show(idEmpty, len(p.visible) == 0)
		p.enable(idRefresh, true)
		p.enable(idConfirm, len(p.selected) > 0)
		p.anchorTopKey, p.anchorFocusKey = "", ""
		return
	}
	if scanDuration > 0 {
		p.lastSnapshot = time.Now()
		p.lastScanDuration = scanDuration
	}
	groups := processcatalog.GroupInstances(instances)
	for index := range groups {
		key := (automation.ProcessTarget{Match: automation.MatchName, Executable: groups[index].Executable}).Key()
		if groups[index].Description != "" {
			p.descriptions[key] = groups[index].Description
		} else if cached := p.descriptions[key]; cached != "" {
			groups[index].Description = cached
		}
	}
	p.items = buildItems(groups, p.selected, p.options.Text)
	p.loading = false
	p.enriching = !final
	p.enable(idRefresh, true)
	p.applyFilter()
	p.restoreViewAnchors(final)
	if p.enriching {
		p.setText(idStatus, p.text("process_picker_loading_descriptions"))
	}
	p.updatePreview()
}

func (p *picker) captureViewAnchors() {
	list := p.controls[idList]
	if list == 0 || len(p.visible) == 0 {
		p.anchorTopKey, p.anchorFocusKey = "", ""
		return
	}
	top, _, _ := pSendMessage.Call(uintptr(list), lvmGetTopIndex, 0, 0)
	if int(top) >= 0 && int(top) < len(p.visible) {
		p.anchorTopKey = p.visible[int(top)].target.Key()
	}
	p.anchorFocusKey = ""
	for index, value := range p.visible {
		state, _, _ := pSendMessage.Call(uintptr(list), lvmGetItemState, uintptr(index), lvisFocused)
		if state&lvisFocused != 0 {
			p.anchorFocusKey = value.target.Key()
			break
		}
	}
}

func (p *picker) restoreViewAnchors(clear bool) {
	indexOf := func(key string) int {
		for index, value := range p.visible {
			if value.target.Key() == key {
				return index
			}
		}
		return -1
	}
	if index := indexOf(p.anchorTopKey); index >= 0 {
		p.scrollListTo(index)
	}
	if index := indexOf(p.anchorFocusKey); index >= 0 {
		entry := lvItem{StateMask: lvisFocused | lvisSelected, State: lvisFocused | lvisSelected}
		pSendMessage.Call(uintptr(p.controls[idList]), lvmSetItemState, uintptr(index), uintptr(unsafe.Pointer(&entry)))
		pSendMessage.Call(uintptr(p.controls[idList]), lvmEnsureVisible, uintptr(index), 0)
	}
	if clear {
		p.anchorTopKey, p.anchorFocusKey = "", ""
	}
}

func processPickerRefreshAge(scanDuration time.Duration) time.Duration {
	age := scanDuration * processPickerScanCostMultiplier
	if age < processPickerAutoRefreshMinAge {
		return processPickerAutoRefreshMinAge
	}
	if age > processPickerAutoRefreshMaxAge {
		return processPickerAutoRefreshMaxAge
	}
	return age
}

func shouldAutoRefreshProcessPicker(last, now time.Time, scanDuration time.Duration, loading bool) bool {
	return !loading && !last.IsZero() && now.Sub(last) >= processPickerRefreshAge(scanDuration)
}

func buildItems(groups []processcatalog.Group, selected map[string]automation.ProcessTarget, text func(string) string) []item {
	items := make([]item, 0, len(groups)+len(selected))
	present := make(map[string]struct{})
	sort.SliceStable(groups, func(i, j int) bool {
		return strings.ToLower(groups[i].Executable) < strings.ToLower(groups[j].Executable)
	})
	for _, group := range groups {
		target := automation.ProcessTarget{Match: automation.MatchName, Executable: group.Executable}
		items = append(items, item{
			target: target, name: group.Executable, description: group.Description,
			count:  strconv.Itoa(group.Count),
			search: strings.ToLower(group.Executable + " " + group.Description),
		})
		present[target.Key()] = struct{}{}
	}
	for key, target := range selected {
		if target.Match == automation.MatchPath {
			continue
		}
		if _, ok := present[key]; ok {
			continue
		}
		description := text("process_picker_not_running")
		items = append(items, item{target: target, name: target.Executable, description: description, search: strings.ToLower(target.Executable + " " + description)})
	}
	return items
}

func (p *picker) applyFilter() {
	filter := strings.ToLower(strings.TrimSpace(p.controlText(idSearch)))
	sorted := sortItems(p.items, p.sortColumn, p.sortAscending)
	next := filterItems(sorted, filter)
	previous := p.visible
	p.visible = next
	p.reconcileVisible(previous, next)
	if len(p.visible) == 0 {
		message := p.text("process_picker_empty")
		if filter != "" {
			message = p.text("process_picker_no_results")
		}
		p.setText(idEmpty, message)
		p.show(idEmpty, true)
	} else {
		p.show(idEmpty, false)
	}
	p.updateSelectionStatus()
}

func (p *picker) reconcileVisible(previous, next []item) {
	list := p.controls[idList]
	if list == 0 {
		return
	}
	p.populating = true
	pSendMessage.Call(uintptr(list), wmSetRedraw, 0, 0)
	if len(next) == 0 || !sameRelativeTargetOrder(previous, next) {
		pSendMessage.Call(uintptr(list), lvmDeleteAllItems, 0, 0)
		for index, value := range next {
			p.insertItem(index, value)
		}
	} else {
		nextKeys := make(map[string]struct{}, len(next))
		for _, value := range next {
			nextKeys[value.target.Key()] = struct{}{}
		}
		retained := make([]item, 0, min(len(previous), len(next)))
		for index := len(previous) - 1; index >= 0; index-- {
			if _, keep := nextKeys[previous[index].target.Key()]; keep {
				continue
			}
			pSendMessage.Call(uintptr(list), lvmDeleteItem, uintptr(index), 0)
		}
		for _, value := range previous {
			if _, keep := nextKeys[value.target.Key()]; keep {
				retained = append(retained, value)
			}
		}
		retainedIndex := 0
		for index, value := range next {
			if retainedIndex < len(retained) && retained[retainedIndex].target.Key() == value.target.Key() {
				p.updateItem(index, retained[retainedIndex], value)
				retainedIndex++
				continue
			}
			p.insertItem(index, value)
		}
	}
	pSendMessage.Call(uintptr(list), wmSetRedraw, 1, 0)
	p.populating = false
	// The vertical scrollbar appears only after rows are populated and reduces
	// the usable report width. Refit at that point so the list never creates a
	// horizontal scrollbar.
	p.syncListScrollbarBounds()
	p.syncListScrollbar()
	p.resizeColumns()
	pShowScrollBar.Call(uintptr(list), sbHorz, 0)
	pRedrawWindow.Call(uintptr(list), 0, 0, rdwInvalidate|rdwErase|rdwAllChildren|rdwUpdateNow|rdwFrame)
}

func sameRelativeTargetOrder(previous, next []item) bool {
	if len(previous) == 0 || len(next) == 0 {
		return true
	}
	nextKeys := make(map[string]struct{}, len(next))
	previousKeys := make(map[string]struct{}, len(previous))
	for _, value := range next {
		nextKeys[value.target.Key()] = struct{}{}
	}
	for _, value := range previous {
		previousKeys[value.target.Key()] = struct{}{}
	}
	commonPrevious := make([]string, 0, min(len(previous), len(next)))
	commonNext := make([]string, 0, cap(commonPrevious))
	for _, value := range previous {
		if _, ok := nextKeys[value.target.Key()]; ok {
			commonPrevious = append(commonPrevious, value.target.Key())
		}
	}
	for _, value := range next {
		if _, ok := previousKeys[value.target.Key()]; ok {
			commonNext = append(commonNext, value.target.Key())
		}
	}
	if len(commonPrevious) == 0 {
		return false
	}
	if len(commonPrevious) != len(commonNext) {
		return false
	}
	for index := range commonPrevious {
		if commonPrevious[index] != commonNext[index] {
			return false
		}
	}
	return true
}

func (p *picker) captureSelection() {
	list := p.controls[idList]
	if list == 0 {
		return
	}
	for index := range p.visible {
		state, _, _ := pSendMessage.Call(uintptr(list), lvmGetItemState, uintptr(index), lvisStateImageMask)
		key := p.visible[index].target.Key()
		if uint32(state)&lvisStateImageMask == 2<<12 {
			p.selected[key] = p.visible[index].target
		} else {
			delete(p.selected, key)
		}
	}
	p.selected = normalizeSelected(p.selected)
	p.syncCheckStates()
	p.updatePreview()
}

func (p *picker) insertItem(index int, value item) {
	columns := []string{value.name, value.description, value.count}
	text, _ := windows.UTF16PtrFromString(columns[0])
	entry := lvItem{Mask: lvifText, Item: int32(index), Text: text}
	pSendMessage.Call(uintptr(p.controls[idList]), lvmInsertItemW, 0, uintptr(unsafe.Pointer(&entry)))
	for column := 1; column < len(columns); column++ {
		value, _ := windows.UTF16PtrFromString(columns[column])
		entry = lvItem{SubItem: int32(column), Text: value}
		pSendMessage.Call(uintptr(p.controls[idList]), lvmSetItemTextW, uintptr(index), uintptr(unsafe.Pointer(&entry)))
	}
	checked := uint32(1 << 12)
	if _, ok := p.selected[value.target.Key()]; ok {
		checked = 2 << 12
	}
	entry = lvItem{StateMask: lvisStateImageMask, State: checked}
	pSendMessage.Call(uintptr(p.controls[idList]), lvmSetItemState, uintptr(index), uintptr(unsafe.Pointer(&entry)))
}

func (p *picker) updateItem(index int, previous, next item) {
	previousColumns := []string{previous.name, previous.description, previous.count}
	nextColumns := []string{next.name, next.description, next.count}
	for column, value := range nextColumns {
		if value == previousColumns[column] {
			continue
		}
		text, _ := windows.UTF16PtrFromString(value)
		entry := lvItem{SubItem: int32(column), Text: text}
		pSendMessage.Call(uintptr(p.controls[idList]), lvmSetItemTextW, uintptr(index), uintptr(unsafe.Pointer(&entry)))
	}
	state := uint32(1 << 12)
	if _, ok := p.selected[next.target.Key()]; ok {
		state = 2 << 12
	}
	entry := lvItem{StateMask: lvisStateImageMask, State: state}
	pSendMessage.Call(uintptr(p.controls[idList]), lvmSetItemState, uintptr(index), uintptr(unsafe.Pointer(&entry)))
}

func (p *picker) syncCheckStates() {
	if p.controls[idList] == 0 {
		return
	}
	p.populating = true
	defer func() { p.populating = false }()
	for index, value := range p.visible {
		state := uint32(1 << 12)
		if _, ok := p.selected[value.target.Key()]; ok {
			state = 2 << 12
		}
		entry := lvItem{StateMask: lvisStateImageMask, State: state}
		pSendMessage.Call(uintptr(p.controls[idList]), lvmSetItemState, uintptr(index), uintptr(unsafe.Pointer(&entry)))
	}
}

func (p *picker) updatePreview() {
	preview := p.controls[idPreview]
	if preview == 0 {
		return
	}
	targets := make([]automation.ProcessTarget, 0, len(p.selected))
	for _, target := range p.selected {
		targets = append(targets, target)
	}
	targets = automation.NormalizeTargets(targets)
	pSendMessage.Call(uintptr(preview), lbResetContent, 0, 0)
	for _, target := range targets {
		name := target.Executable
		if description := p.descriptions[target.Key()]; description != "" {
			name = fmt.Sprintf(p.text("process_name_description"), name, description)
		}
		label := fmt.Sprintf(p.text("process_picker_preview_name"), name)
		if target.Match == automation.MatchPath {
			label = fmt.Sprintf(p.text("process_picker_preview_path"), name, target.Path)
		}
		text, _ := windows.UTF16PtrFromString(label)
		pSendMessage.Call(uintptr(preview), lbAddString, 0, uintptr(unsafe.Pointer(text)))
	}
	if len(targets) == 0 {
		text, _ := windows.UTF16PtrFromString(p.text("process_picker_preview_empty"))
		pSendMessage.Call(uintptr(preview), lbAddString, 0, uintptr(unsafe.Pointer(text)))
	}
	p.setText(idPreviewTitle, fmt.Sprintf(p.text("process_picker_selection_title"), len(targets)))
	p.enable(idConfirm, len(targets) > 0 && len(targets) <= automation.MaxProcessesPerRule)
	if p.previewScroll != nil {
		p.previewScroll.Sync()
	}
}

func filterItems(values []item, filter string) []item {
	if filter == "" {
		return append([]item(nil), values...)
	}
	out := make([]item, 0, len(values))
	for _, value := range values {
		if strings.Contains(value.search, filter) {
			out = append(out, value)
		}
	}
	return out
}

func sortItems(values []item, column int, ascending bool) []item {
	out := append([]item(nil), values...)
	valueAt := func(value item) string {
		switch column {
		case 1:
			return value.description
		case 2:
			count, _ := strconv.Atoi(value.count)
			return fmt.Sprintf("%08d", count)
		default:
			return value.name
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		left := strings.ToLower(valueAt(out[i]))
		right := strings.ToLower(valueAt(out[j]))
		if ascending {
			return left < right
		}
		return left > right
	})
	return out
}

func normalizeSelected(values map[string]automation.ProcessTarget) map[string]automation.ProcessTarget {
	targets := make([]automation.ProcessTarget, 0, len(values))
	for _, target := range values {
		targets = append(targets, target)
	}
	normalized := automation.NormalizeTargets(targets)
	out := make(map[string]automation.ProcessTarget, len(normalized))
	for _, target := range normalized {
		out[target.Key()] = target
	}
	return out
}

func (p *picker) updateSelectionStatus() {
	if len(p.selected) > automation.MaxProcessesPerRule {
		p.setText(idStatus, fmt.Sprintf(p.text("process_picker_limit"), automation.MaxProcessesPerRule))
		return
	}
	p.setText(idStatus, fmt.Sprintf(p.text("process_picker_status_results"), len(p.visible), len(p.selected)))
}

func canAddSelection(values map[string]automation.ProcessTarget, target automation.ProcessTarget) bool {
	if _, exists := values[target.Key()]; exists {
		return true
	}
	targets := make([]automation.ProcessTarget, 0, len(values)+1)
	for _, existing := range values {
		targets = append(targets, existing)
	}
	targets = append(targets, target)
	return len(automation.NormalizeTargets(targets)) <= automation.MaxProcessesPerRule
}

func (p *picker) confirm() {
	p.captureSelection()
	if len(p.selected) > automation.MaxProcessesPerRule {
		p.updateSelectionStatus()
		return
	}
	targets := make([]automation.ProcessTarget, 0, len(p.selected))
	for _, target := range p.selected {
		targets = append(targets, target)
	}
	targets = automation.NormalizeTargets(targets)
	descriptions := make(map[string]string, len(targets))
	for _, target := range targets {
		if description := p.descriptions[target.Key()]; description != "" {
			descriptions[target.Key()] = description
		}
	}
	callback := p.options.OnConfirm
	p.destroy()
	if callback != nil {
		callback(targets, descriptions)
	}
}

func (p *picker) browseExecutable() {
	filter := utf16.Encode([]rune(p.text("process_picker_exe_filter") + "\x00*.exe\x00\x00"))
	file := make([]uint16, 32768)
	title, _ := windows.UTF16PtrFromString(p.text("process_picker_browse_title"))
	defaultExtension, _ := windows.UTF16PtrFromString("exe")
	dialog := openFileName{
		Size: uint32(unsafe.Sizeof(openFileName{})), Owner: p.hwnd,
		Filter: &filter[0], FilterIndex: 1, File: &file[0], MaxFile: uint32(len(file)),
		Title: title, DefaultExtension: defaultExtension,
		Flags: ofnHideReadOnly | ofnNoChangeDir | ofnPathMustExist | ofnFileMustExist | ofnExplorer | ofnDontAddToRecent,
	}
	chosen, _, _ := pGetOpenFileName.Call(uintptr(unsafe.Pointer(&dialog)))
	if chosen == 0 {
		return
	}
	path := filepath.Clean(windows.UTF16ToString(file))
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil || !strings.EqualFold(filepath.Ext(path), ".exe") {
		p.setText(idStatus, p.text("process_picker_invalid_exe"))
		return
	}
	var binaryType uint32
	valid, _, _ := pGetBinaryType.Call(uintptr(unsafe.Pointer(pathPtr)), uintptr(unsafe.Pointer(&binaryType)))
	if valid == 0 {
		p.setText(idStatus, p.text("process_picker_invalid_exe"))
		return
	}
	target := automation.ProcessTarget{Match: automation.MatchPath, Executable: filepath.Base(path), Path: path}
	candidate := make(map[string]automation.ProcessTarget, len(p.selected)+1)
	for key, existing := range p.selected {
		candidate[key] = existing
	}
	for key, existing := range candidate {
		if existing.Match == automation.MatchName && strings.EqualFold(existing.Executable, target.Executable) {
			delete(candidate, key)
		}
	}
	if !canAddSelection(candidate, target) {
		p.setText(idStatus, fmt.Sprintf(p.text("process_picker_limit"), automation.MaxProcessesPerRule))
		return
	}
	candidate[target.Key()] = target
	p.selected = normalizeSelected(candidate)
	p.syncCheckStates()
	p.updatePreview()
	p.updateSelectionStatus()
	key := target.Key()
	p.workers.Add(1)
	go func() {
		defer p.workers.Done()
		description := fileDescriptionForPicker(path)
		postPickerUI(func() {
			if p.closed.Load() || p.hwnd == 0 {
				return
			}
			p.descriptionTried[key] = struct{}{}
			if description != "" {
				p.descriptions[key] = description
			}
			p.updatePreview()
		})
	}()
}
