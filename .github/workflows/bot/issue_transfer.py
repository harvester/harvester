import re
import logging

from flask import render_template

from bot import repo


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