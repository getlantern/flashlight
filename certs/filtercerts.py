_noncert_extensions = ".yaml", ".txt", ".py", ".pyc", ".tmpl", ".bash", ".swp"

def iscert(filename):
	extension = filename[filename.rfind('.'):]
	return extension not in _noncert_extensions
