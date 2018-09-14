package android

func Debug(tag, msg string) {
	log.Infof("%s: %s", tag, msg)
}

func Error(tag, msg string) {
	log.Errorf("%s: %s", tag, msg)
}
