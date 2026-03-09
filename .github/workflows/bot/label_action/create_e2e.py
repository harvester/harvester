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

                issue_test_data = {
                    'title': issue_test_title,
                    'body': issue_test_body
                }

                milestone = self._get_milestone(issue)
                if milestone:
                    issue_test_data['milestone'] = milestone

                issue_test = repo_test.create_issue(**issue_test_data)

                issue_test_link = '{}/{}#{}'.format(GITHUB_OWNER, GITHUB_REPOSITORY_TEST, issue_test.number)

                # link test issue in Harvester issue
                issue.create_comment('Automation e2e test issue: {}'.format(issue_test_link))
        else:
            logging.info('label {} does not exist, not creating test issue'.format(target_label))

    def _get_milestone(self, issue):
        """
        Get the milestone from Harvester issue and find or create it in test repo.

        Args:
            issue: The Harvester issue object

        Returns:
            The milestone object from test repo, or None if no milestone should be set
        """
        if issue.milestone is None:
            logging.info('No milestone set on Harvester issue, skipping milestone assignment')
            return None

        milestone_title = issue.milestone.title
        # Skip if milestone is empty or "planning" (case insensitive)
        if not milestone_title or milestone_title.lower() == 'planning':
            logging.info('Milestone is empty or "planning", skipping milestone assignment')
            return None

        # Find or create the same milestone in test repo
        test_milestone = self._find_or_create_milestone(milestone_title, issue.milestone)
        if test_milestone:
            logging.info(f'Set milestone "{milestone_title}" for e2e issue')
            return test_milestone
        else:
            logging.warning(f'Failed to find or create milestone "{milestone_title}" in test repo')
            return None

    def _find_or_create_milestone(self, milestone_title, harvester_milestone):
        """
        Find or create a milestone in the test repo that matches the Harvester milestone.

        Args:
            milestone_title: The title of the milestone to find/create
            harvester_milestone: The original Harvester milestone object

        Returns:
            The milestone object from test repo, or None if failed
        """
        try:
            # First, try to find existing milestone in test repo
            milestones = repo_test.get_milestones(state='all')
            for ms in milestones:
                if ms.title == milestone_title:
                    logging.info(f'Found existing milestone "{milestone_title}" in test repo')
                    return ms

            # If not found, create a new milestone in test repo
            logging.info(f'Milestone "{milestone_title}" not found in test repo, creating new one')

            milestone_data = {
                'title': milestone_title,
            }

            if harvester_milestone.description:
                milestone_data['description'] = harvester_milestone.description

            if harvester_milestone.due_on:
                milestone_data['due_on'] = harvester_milestone.due_on

            new_milestone = repo_test.create_milestone(**milestone_data)
            logging.info(f'Created new milestone "{milestone_title}" in test repo')
            return new_milestone

        except Exception as e:
            logging.error(f'Error finding or creating milestone "{milestone_title}": {str(e)}')
            return None
