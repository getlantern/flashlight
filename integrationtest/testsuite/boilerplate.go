package testsuite

type boilerplateTest struct {
	status TestStatus
}

func (t *boilerplateTest) setStatus(s TestStatus) {
	t.status = s
	updateChan <- struct{}{}
}
