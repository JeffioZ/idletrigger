package monitor

import "time"

func (m *Monitor) observeInputTimestamp() (available, sessionReset bool) {
	tick, err := m.inputTimestamp()
	if err != nil {
		return false, false
	}
	if m.seenInputTick && tick != m.lastInputTick {
		wasWarned := m.warned.Load()
		wasTriggered := m.triggered.Load()
		previousTick := m.lastInputTick
		interval := elapsedSinceLastInput(tick, previousTick)
		threshold, warnAt := m.thresholdWindow()
		ignored, reason := m.classifyInputReset(interval)
		if m.onInputReset != nil {
			m.onInputReset(InputReset{
				PreviousLastInputTick:  previousTick,
				LastInputTick:          tick,
				SessionIdleBeforeReset: interval,
				Threshold:              threshold,
				WarnAt:                 warnAt,
				WasWarned:              wasWarned,
				WasTriggered:           wasTriggered,
				Ignored:                ignored,
				Reason:                 reason,
				PeriodicCount:          m.periodic.count,
				PeriodicBaseline:       m.periodic.baseline,
			})
		}
		m.lastInputTick = tick
		if ignored {
			m.periodic.useLogicalIdle = true
		} else {
			m.resetSession()
			if wasWarned && m.onActivity != nil {
				m.onActivity()
			}
			return true, true
		}
	}
	m.lastInputTick = tick
	m.seenInputTick = true
	return true, false
}

func (m *Monitor) sample(inputTimestampAvailable bool) (Sample, error) {
	idle, err := m.idleDuration()
	if err != nil {
		return Sample{}, err
	}
	// GetLastInputInfo reports activity that predates this monitor. A newly
	// launched or re-enabled monitor must begin its own timeout window rather
	// than immediately act on that historic idle period.
	clamped := false
	sinceStart := time.Since(m.startedAt)
	if m.periodic.useLogicalIdle && m.ignorePeriodic.Load() {
		idle = sinceStart
	} else if idle > sinceStart {
		idle = sinceStart
		clamped = true
	}
	threshold, warnAt := m.thresholdWindow()
	return Sample{
		Idle:                    idle,
		Threshold:               threshold,
		WarnAt:                  warnAt,
		InputTimestampAvailable: inputTimestampAvailable,
		LastInputTick:           m.lastInputTick,
		StartWindowClamped:      clamped,
		Warned:                  m.warned.Load(),
		Triggered:               m.triggered.Load(),
	}, nil
}

func (m *Monitor) thresholdWindow() (threshold, warnAt time.Duration) {
	threshold = time.Duration(m.thresholdNs.Load())
	warnAt = threshold - m.warningOffset
	if warnAt < 0 {
		warnAt = 0
	}
	return threshold, warnAt
}

func (m *Monitor) evaluateSample(sample Sample) {
	if sample.Idle >= sample.WarnAt && sample.Idle < sample.Threshold {
		if !m.warned.Swap(true) && m.onWarning != nil {
			m.onWarning()
		}
	}
	if sample.Idle >= sample.Threshold {
		if !m.triggered.Swap(true) && m.onTrigger != nil {
			m.onTrigger()
			// An accepted action ends this idle session. Start a fresh timeout
			// window so unlocking cannot immediately retrigger on stale input.
			m.resetSession()
		}
	}
	if !sample.InputTimestampAvailable && sample.Idle < sample.WarnAt {
		if m.warned.Swap(false) && m.onActivity != nil {
			m.onActivity()
		}
		m.triggered.Store(false)
	}
}
