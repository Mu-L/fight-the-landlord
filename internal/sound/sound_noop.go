//go:build ci

package sound

type SoundManager struct{}

func NewSoundManager() *SoundManager {
	return &SoundManager{}
}

func (sm *SoundManager) Init() error {
	return nil
}

func (sm *SoundManager) Play(name string) {
	// No-op
}

func (sm *SoundManager) PlaySequence(names ...string) {
	// No-op
}

func (sm *SoundManager) PlayBGM(name string) {
	// No-op
}

func (sm *SoundManager) PlayBGMAnyOf(names ...string) {
	// No-op
}

func (sm *SoundManager) StopBGM() {
	// No-op
}

func (sm *SoundManager) Muted() bool {
	return true
}

func (sm *SoundManager) ToggleMute() bool {
	return true
}

func (sm *SoundManager) Close() {
	// No-op
}
