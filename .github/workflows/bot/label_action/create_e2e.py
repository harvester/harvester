from bot import repo
from bot.action import LabelAction
import re
import logging
from bot import repo, repo_test, \
    GITHUB_OWNER, GITHUB_REPOSITORY, GITHUB_REPOSITORY_TEST
    
template_re = re.compile('---\n.*?---\n', re.DOTALL)

target_label = 'require/auto-e2e-test'

class CreateE2EIssue(LabelAction):
    def __init__(self):
        pass
    
    def isMatched(self, request):
        for label in request['issue']['labels']:
            if label['name'] == target_label:
                return True
        return False
    
    def action(self, request):
        require_e2e = False
        issue = repo.get_issue(request['issue']['number'])
        
        labels = issue.get_labels()
        comments = issue.get_comments() 
        
        for label in labels:
            if label.name == target_label:
                require_e2e = True
                break

        if require_e2e:
            found = False
            for comment in comments:
                if comment.body.startswith('Automation e2e test issue:'):
                    logging.info('Automation e2e test issue already exists, not creating a new one')
                    found = True
                    break
            if not found:
                logging.info('Automation e2e test issue does not exist, creating a new one')

                issue_link = '{}/{}#{}'.format(GITHUB_OWNER, GITHUB_REPOSITORY, issue.number)
                issue_test_title = '[e2e] {}'.format(issue.title)
                issue_test_template_content = repo_test.get_contents(
                    ".github/ISSUE_TEMPLATE/test.md").decoded_content.decode()
                issue_test_body = template_re.sub("\n", issue_test_template_content, count=1)
                issue_test_body += '\nrelated issue: {}'.format(issue_link)
                issue_test = repo_test.create_issue(title=issue_test_title, body=issue_test_body)

                issue_test_link = '{}/{}#{}'.format(GITHUB_OWNER, GITHUB_REPOSITORY_TEST, issue_test.number)

                # link test issue in Harvester issue
                issue.create_comment('Automation e2e test issue: {}'.format(issue_test_link))
        else:
            logging.info('label {} does not exist, not creating test issue'.format(target_label))