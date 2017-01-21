import tempfile
import os
import time
from os.path import join

import requests
from requests.auth import HTTPDigestAuth
from requests.cookies import RequestsCookieJar
from requests.exceptions import  ConnectionError

from gerrittest.logger import logger
from gerrittest.command import check_output


def create_admin(address, port):
    """Creates the admin account and returns (username, secret)"""
    logger.debug("Creating admin account.")
    cookies = RequestsCookieJar()
    base_url = "http://{address}:{port}".format(address=address, port=port)

    url = "{}/login/%23%2F?account_id=1000000".format(base_url)
    response = requests.get(url, cookies=cookies)
    logger.debug("GET %s (response: %s)", url, response.status_code)
    response.raise_for_status()

    # Try to login with the newly created admin account.
    url = "{}/a/accounts/self".format(base_url)
    response = requests.get(url, auth=HTTPDigestAuth("admin", "secret"))
    logger.debug("GET %s (response: %s)", url, response.status_code)
    response.raise_for_status()

    return "admin", "secret"


def generate_rsa_key():
    """Generates an RSA key for ssh. Returns the generated key."""
    logger.debug("Generating RSA key.")
    dirname = tempfile.mkdtemp()
    path = join(dirname, "id_rsa")

    # TODO Figure out why this only works with os.system. With check_output
    # ssh-keygen basically ignore the -q/-N flags even with shell=True
    command = 'ssh-keygen -b 2048 -t rsa -f %s -q -N ""' % path
    logger.debug(command)
    os.system(command)
    return path


def add_rsa_key(address, http_port, ssh_port, username, password, key_path):
    """
    Adds the ssh key provided by ``key_path`` to the requested user's
    account.
    """
    logger.debug("Adding RSA key %s to %s", key_path, username)
    url = "http://{address}:{port}/a/accounts/self/sshkeys".format(
        address=address, port=http_port)
    cookies = RequestsCookieJar()

    logger.debug("POST %s", url)
    with open(key_path + ".pub", "rb") as key:
        response = requests.post(
            url,
            data=key, cookies=cookies,
            auth=HTTPDigestAuth(username, password))
        response.raise_for_status()

    check_output([
        "ssh",
        "-o", "LogLevel=quiet",
        "-o", "UserKnownHostsFile=%s" % os.devnull,
        "-o", "StrictHostKeyChecking=no",
        "-i", key_path,
        "-p", str(ssh_port),
        "admin@%s" % address,
        "gerrit", "version"
    ])
