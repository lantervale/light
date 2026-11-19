"""light as a product of wptrunner: `./wpt run light`.

light is its own WebDriver server (`light drive --address HOST:PORT`), so the
binary and the WebDriver binary are one. It finds the test servers' names
on this machine by itself (`--host-map`) and trusts their authority for
the run (`--trust-ca`): neither /etc/hosts nor the system's trust store is
touched.
"""

import sys

from wptrunner.browsers.base import WebDriverBrowser, get_timeout_multiplier, require_arg
from wptrunner.executors import executor_kwargs as base_executor_kwargs
from wptrunner.executors.executorwebdriver import (
    WebDriverRefTestExecutor,
    WebDriverTestharnessExecutor,
)
from wptrunner.products import Product

NAME = "light"

# How long one script of a test may run, in seconds, before the runner's
# timeout multiplier: a test is one long task more often than a page is.
SCRIPT_BUDGET = 10


class LightBrowser(WebDriverBrowser):
    def __init__(self, logger, binary, hosts, ca=None, script_budget=SCRIPT_BUDGET, **kwargs):
        super().__init__(logger, binary=binary, webdriver_binary=binary, supports_pac=False, **kwargs)
        self.hosts = hosts
        self.ca = ca
        self.script_budget = script_budget

    def make_command(self):
        command = [self.binary, "drive", "--address", f"{self.host}:{self.port}"]
        if self.hosts:
            command += ["--host-map", ";".join(f"{name}=127.0.0.1" for name in self.hosts)]
        if self.ca:
            command += ["--trust-ca", self.ca]
        command += ["--script-budget", f"{self.script_budget:g}s"]
        return command + self.webdriver_args


def check_args(**kwargs):
    require_arg(kwargs, "binary")


def browser_kwargs(logger, test_type, run_info_data, config, **kwargs):
    secure = kwargs.get("ssl_type") != "none"
    return {
        "binary": kwargs["binary"],
        "webdriver_args": kwargs.get("webdriver_args"),
        "hosts": sorted(names[""] for names in config.domains.values()),
        "ca": config.ssl_config.get("ca_cert_path") if secure else None,
        "script_budget": SCRIPT_BUDGET * get_timeout_multiplier(test_type, run_info_data, **kwargs),
    }


def executor_kwargs(logger, test_type, test_environment, run_info_data, **kwargs):
    rv = base_executor_kwargs(test_type, test_environment, run_info_data, **kwargs)
    rv["capabilities"] = {}
    return rv


def env_extras(**kwargs):
    return []


def without_hosts_file():
    """`wpt run` refuses a product it does not know until /etc/hosts names
    the test servers. light needs no such line, so that one check is
    skipped for it, in this process, and kept for every other product."""
    run = sys.modules.get("tools.wpt.run")
    if run is None or getattr(run.check_environ, "light", False):
        return
    check = run.check_environ

    def check_environ(product):
        if product != NAME:
            check(product)

    check_environ.light = True
    run.check_environ = check_environ


def get_product():
    without_hosts_file()
    return Product(
        name=NAME,
        browser_classes={None: LightBrowser},
        check_args=check_args,
        get_browser_kwargs=browser_kwargs,
        get_executor_kwargs=executor_kwargs,
        env_options={"server_host": "127.0.0.1"},
        get_env_extras=env_extras,
        get_timeout_multiplier=get_timeout_multiplier,
        executor_classes={
            "testharness": WebDriverTestharnessExecutor,
            "reftest": WebDriverRefTestExecutor,
        },
    )
