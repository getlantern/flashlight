#!/usr/bin/env python

import os

from addmasquerades import addmasquerades
from filtercerts import iscert


def refreshcerts():
	addmasquerades([name for name in os.listdir('.') if iscert(name)],
				   refreshcerts=True)


if __name__ == '__main__':
	refreshcerts()
