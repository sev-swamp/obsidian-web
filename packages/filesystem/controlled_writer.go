package filesystem

// EventSuppressor registers the exact final state expected from a controlled
// filesystem operation. The returned function cancels a registration when the
// write did not happen.
type EventSuppressor interface {
	Suppress(path string, expected FileState) (cancel func())
}

// ControlledWriter performs durable sync writes and tells the watcher which
// matching notifications belong to that write. It does not publish domain
// events; its caller must do that after completing its own durable journal.
type ControlledWriter struct {
	vault      *Vault
	suppressor EventSuppressor
}

func NewControlledWriter(vault *Vault, suppressor EventSuppressor) *ControlledWriter {
	return &ControlledWriter{vault: vault, suppressor: suppressor}
}

func (w *ControlledWriter) Write(path string, data []byte) error {
	p, err := w.vault.CanonicalPath(path)
	if err != nil {
		return err
	}
	cancel := func() {}
	if w.suppressor != nil {
		cancel = w.suppressor.Suppress(p, fileState(data))
	}
	if err := w.vault.WriteAtomic(p, data); err != nil {
		cancel()
		return err
	}
	return nil
}

func (w *ControlledWriter) Delete(path string) error {
	p, err := w.vault.CanonicalPath(path)
	if err != nil {
		return err
	}
	cancel := func() {}
	if w.suppressor != nil {
		cancel = w.suppressor.Suppress(p, FileState{})
	}
	if err := w.vault.DeleteAtomic(p); err != nil {
		cancel()
		return err
	}
	return nil
}
