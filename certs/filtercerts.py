_noncert_extensions = [".yaml", ".txt", ".py", ".pyc", ".tmpl", ".bash",
		               ".swp", ".swo"]

def iscert(filename):
	extension = filename[filename.rfind('.'):]
	return extension not in _noncert_extensions
