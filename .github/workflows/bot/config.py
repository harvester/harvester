from everett.component import RequiredConfigMixin, ConfigOptions
from everett.manager import ConfigManager, ConfigOSEnv
from github import Github
from bot.github_graphql.manager import GitHubProjectManager
from bot.global_variables import *

GITHUB_OWNER = ""
GITHUB_REPOSITORY = ""
GITHUB_REPOSITORY_TEST = ""
GITHUB_PROJECT_NUMBER = ""
E2E_PIPELINE = ""
BACKPORT_LABEL_KEY = ""


class BotConfig(RequiredConfigMixin):
    required_config = ConfigOptions()
    required_config.add_option('github_owner', parser=str, default='harvester',
                               doc='Set the owner of the target GitHub '
                                   'repository.')
    required_config.add_option('github_main_repository', parser=str, default='harvester', doc='Set the name of the target '
                                                                                         'GitHub repository.')
    required_config.add_option('github_repository_test', parser=str, default='tests', doc='Set the name of the tests '
                                                                                          'GitHub repository.')
    required_config.add_option('github_project_number', parser=int, doc='Set the project id of the github '
                                                                                          'GitHub Project ID.')
    required_config.add_option('github_token', parser=str, doc='Set the token of the GitHub machine user.')
    required_config.add_option('e2e_pipeline', parser=str, default='Review,Ready For Testing,Testing',
                               doc='Set the target e2e pipeline to '
                                   'handle events for.')
    required_config.add_option('backport_label_key', parser=str, default='backport-needed',
                               doc='Set the backport label key.')


def get_config():
    c = ConfigManager(environments=[
        ConfigOSEnv()
    ])
    return c.with_options(BotConfig())


def settings():
    global GITHUB_OWNER, GITHUB_REPOSITORY, GITHUB_PROJECT_NUMBER, GITHUB_REPOSITORY_TEST, \
        E2E_PIPELINE, BACKPORT_LABEL_KEY, gh_api, zenh_api, repo, repo_test, gtihub_project_manager
    config = get_config()
    GITHUB_OWNER = config('github_owner')
    GITHUB_REPOSITORY = config('github_main_repository')
    GITHUB_REPOSITORY_TEST = config('github_repository_test')
    GITHUB_PROJECT_NUMBER = config('github_project_number')
    E2E_PIPELINE = config('e2e_pipeline')
    BACKPORT_LABEL_KEY = config('backport_label_key', default='backport-needed')

    gh_api = Github(config('github_token'))
    repo = gh_api.get_repo('{}/{}'.format(GITHUB_OWNER, GITHUB_REPOSITORY))
    repo_test = gh_api.get_repo('{}/{}'.format(GITHUB_OWNER, GITHUB_REPOSITORY_TEST))
    gtihub_project_manager = GitHubProjectManager(GITHUB_OWNER, GITHUB_REPOSITORY, GITHUB_PROJECT_NUMBER, {
        'Authorization': f'Bearer {config("github_token")}',
        'Content-Type': 'application/json'
    })

settings()