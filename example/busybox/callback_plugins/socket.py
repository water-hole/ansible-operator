from __future__ import (absolute_import, division, print_function)
__metaclass__ = type

DOCUMENTATION = '''
    callback: socket
    type: notification
    short_description: send playbook output to socket
    version_added: 2.7
    description:
      - This callback writes playbook output to a socket (passed via the CALLBACK_SOCKET environment variable)
    requirements:
     - Whitelist in configuration
     - A socket to send the logs to
'''

import os
import time
import json
import socket
from collections import MutableMapping

from ansible.module_utils._text import to_bytes
from ansible.plugins.callback import CallbackBase


# NOTE: in Ansible 1.2 or later general logging is available without
# this plugin, just set ANSIBLE_LOG_PATH as an environment variable
# or log_path in the DEFAULTS section of your ansible configuration
# file.  This callback is an example of per hosts logging for those
# that want it.


class CallbackModule(CallbackBase):
    """
    logs playbook results, per host, in a socket given via CALLBACK_SOCKET
    """
    CALLBACK_VERSION = 2.0
    CALLBACK_TYPE = 'notification'
    CALLBACK_NAME = 'socket'
    CALLBACK_NEEDS_WHITELIST = True

    TIME_FORMAT = "%b %d %Y %H:%M:%S"
    MSG_FORMAT = "%(data)s\n\n"

    def __init__(self):

        super(CallbackModule, self).__init__()
        self.socket_path = os.environ.get('CALLBACK_SOCKET')
        if not self.socket_path or not os.path.exists(self.socket_path):
            raise Exception("Socket %s does not exist, pass in a valid path to a socket via CALLBACK_SOCKET".format(self.socket.path))
        self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.sock.connect(self.socket_path)


    def log(self, host, category, data):
        if isinstance(data, MutableMapping):
            data = data.copy()
            now = time.strftime(self.TIME_FORMAT, time.localtime())
            data['status'] = category
            data['host'] = host
            data['time'] = now
            data = json.dumps(data)
        else:
            self._display.vvvv("return")
            return


        self._display.vvvv("here")
        msg = to_bytes("{}\n".format(data))
        self.sock.sendall(msg)

    def endlog(self):
        self._display.vvvv("end log here")
        msg = to_bytes("endfile\n")
        self.sock.sendall(msg)


    def runner_on_failed(self, host, res, ignore_errors=False):
        self.log(host, 'FAILED', res)

    def runner_on_ok(self, host, res):
        self.log(host, 'OK', res)

    def runner_on_skipped(self, host, item=None):
        self.log(host, 'SKIPPED', '...')

    def runner_on_unreachable(self, host, res):
        self.log(host, 'UNREACHABLE', res)

    def runner_on_async_failed(self, host, res, jid):
        self.log(host, 'ASYNC_FAILED', res)

    def playbook_on_import_for_host(self, host, imported_file):
        self.log(host, 'IMPORTED', imported_file)

    def playbook_on_not_import_for_host(self, host, missing_file):
        self.log(host, 'NOTIMPORTED', missing_file)

    def playbook_on_stats(self, stats):
        self.log("", "OK", stats)
        self.endlog()
