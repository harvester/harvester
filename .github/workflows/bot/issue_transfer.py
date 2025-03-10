import re
import logging

from flask import render_template

from bot import repo, repo_test, \
    GITHUB_OWNER, GITHUB_REPOSITORY, GITHUB_REPOSITORY_TEST


template_re = re.compile('---\n.*?---\n', re.DOTALL)

class IssueTransfer:
    def __init__(
            self,
            issue_number
    ):
        self.__comments = None
        self.__issue = repo.get_issue(issue_number)

    def create_comment_if_not_exist(self):
        self.__comments = self.__issue.get_comments()
        found = False
        for comment in self.__comments:
            if comment.body.strip().startswith('## Pre Ready-For-Testing Checklist'):
                logging.info('pre-merged checklist already exists, not creating a new one')
                found = True
                break
        if not found:
            logging.info('pre-merge checklist does not exist, creating a new one')
            self.__issue.create_comment(render_template('pre-merge.md'))

    def create_e2e_issue(self):
        require_e2e = True
        labels = self.__issue.get_labels()
        for label in labels:
            if label.name == 'not-require/test-plan':
                require_e2e = False
                break
        if require_e2e:
            found = False
            for comment in self.__comments:
                if comment.body.startswith('Automation e2e test issue:'):
                    logging.info('Automation e2e test issue already exists, not creating a new one')
                    found = True
                    break
            if not found:
                logging.info('Automation e2e test issue does not exist, creating a new one')

                issue_link = '{}/{}#{}'.format(GITHUB_OWNER, GITHUB_REPOSITORY, self.__issue.number)
                issue_test_title = '[e2e] {}'.format(self.__issue.title)
                issue_test_template_content = repo_test.get_contents(
                    ".github/ISSUE_TEMPLATE/test.md").decoded_content.decode()
                issue_test_body = template_re.sub("\n", issue_test_template_content, count=1)
                issue_test_body += '\nrelated issue: {}'.format(issue_link)
                issue_test = repo_test.create_issue(title=issue_test_title, body=issue_test_body)

                issue_test_link = '{}/{}#{}'.format(GITHUB_OWNER, GITHUB_REPOSITORY_TEST, issue_test.number)

                # link test issue in Harvester issue
                self.__issue.create_comment('Automation e2e test issue: {}'.format(issue_test_link))
        else:
            logging.info('label require/automation-e2e does not exists, not creating test issue')
