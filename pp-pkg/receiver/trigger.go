package receiver

type ReloadBlocksTriggerNotifier struct {
	c chan struct{}
}

func NewReloadBlocksTriggerNotifier() *ReloadBlocksTriggerNotifier {
	return &ReloadBlocksTriggerNotifier{c: make(chan struct{}, 1)}
}

func (tn *ReloadBlocksTriggerNotifier) Chan() <-chan struct{} {
	return tn.c
}

func (tn *ReloadBlocksTriggerNotifier) NotifyWritten() {
	select {
	case tn.c <- struct{}{}:
	default:
	}
}
