import importlib
import os


class EnvironmentSetup:
    def __init__(self, repo_url, git_credentials, mongo_uri, sudopass=None, user=None, smtp_host=None, smtp_port=None, smtp_user=None, smtp_pass=None,admin_email=None):
        self.repo_url = repo_url
        self.git_credentials = git_credentials
        self.mongodb_uri = mongo_uri
        self.sudo_password = sudopass
        self.user = user
        self.smtp_host = smtp_host
        self.smtp_port = smtp_port
        self.smtp_user = smtp_user
        self.smtp_pass = smtp_pass
        self.admin_email=admin_email

    def install_dependencies(self):
        raise NotImplementedError("This method should be implemented by subclasses")

    def clone_code(self):
        raise NotImplementedError("This method should be implemented by subclasses")

    def setup_server(self):
        raise NotImplementedError("This method should be implemented by subclasses")

    def start_monitoring(self):
        raise NotImplementedError("This method should be implemented by subclasses")

# Plugin registry
plugins = {}

def register_plugin(name):
    def decorator(cls):
        plugins[name] = cls
        return cls
    return decorator
# Dynamically import all modules in the plugins folder
plugin_folder = os.path.dirname(__file__)
for filename in os.listdir(plugin_folder):
    if filename.endswith(".py") and filename != "__init__.py":
        module_name = filename[:-3]
        importlib.import_module(f'plugins.{module_name}')