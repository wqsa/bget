package tracker

type AnnounceStats struct {
	Download int64
	Upload   int64
	Left     int64
}

type options struct {
	event Event
	stats AnnounceStats
}

type Option interface {
	apply(*options)
}

type eventOption Event

func (e eventOption) apply(o *options) {
	o.event = Event(e)
}

type statsOption AnnounceStats

func (s statsOption) apply(o *options) {
	o.stats = AnnounceStats(s)
}

func WithEvent(event Event) Option {
	return eventOption(event)
}

func WithStats(stats AnnounceStats) Option {
	return statsOption(stats)
}
