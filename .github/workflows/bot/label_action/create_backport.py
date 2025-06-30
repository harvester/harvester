import re, logging
from bot import repo, BACKPORT_LABEL_KEY
from bot.exception import CustomException, ExistedBackportComment
from bot.label_action.create_gui_issue import CREATE_GUI_ISSUE_LABEL
from bot.action import LabelAction

# check the issue's include backport-needed/(1.0.3|v1.0.3|v1.0.3-rc0) label
backport_label_pattern = r'^%s\/[\w0-9\.]+' % BACKPORT_LABEL_KEY


# link: https://github.com/harvester/harvester/issues/2324
# Title: [Backport v1.x] copy-the-title.
# Description: backport the issue #link-id
# Copy assignees and all labels except the backport-needed.
# Move the issue to the associated milestone and release.
class CreateBackport(LabelAction):
    def __init__(self):
        pass
    
    def isMatched(self, request):
        for label in request['issue']['labels']:
            if re.match(backport_label_pattern, label['name']) is not None:
                return True
        return False
                    
    def action(self, request):
        normal_labels = []
        backport_labels = []
        for label in request['issue']['labels']:
            if re.match(backport_label_pattern, label['name']) is not None:
                backport_labels.append(label)
            else:
                # backport should not include the 'require/ui' label
                # because gui issue has its own backport
                if CREATE_GUI_ISSUE_LABEL not in label['name']:
                    normal_labels.append(label)

        msg = []
        for backport_label in backport_labels:
            try:
                logging.info(f"issue number {request['issue']['number']} start to create backport with labels {backport_label['name']}")
                
                bp = Backport(request['issue']['number'], normal_labels, backport_label)
                bp.verify()
                bp.create_issue_if_not_exist()
                bp.create_comment()
                msg.append("create backport issue success")
            except ExistedBackportComment as e:
                logging.info(f"issue number {request['issue']['number']} had created backport with labels {backport_label['name']}")
            except CustomException as e:
                logging.exception(f"Custom exception : {str(e)}")
            except Exception as e:
                logging.exception(e)
        
        return ",".join(msg)

class Backport:
    def __init__(
            self,
            issue_number,
            labels,
            backport_label,
    ):
        self.__issue = None
        self.__origin_issue = repo.get_issue(issue_number)
        self.__labels = [repo.get_label(label["name"]) for label in labels]

        self.__backport_needed = repo.get_label(backport_label["name"])
        self.__ver = ""
        self.__parse_ver()

        self.__milestone = None
        self.__parse_milestone()

    def __parse_ver(self):
        self.__ver = self.__backport_needed.name.split("/")[1]

        if self.__ver == "":
            return
        
        if not self.__ver.startswith("v"):
            self.__ver = "v" + self.__ver

    def __parse_milestone(self):
        if self.__ver == "":
            return

        milestones = repo.get_milestones(state='open')
        for ms in milestones:
            if ms.title == self.__ver:
                self.__milestone = ms
                break

    def verify(self):
        if self.__ver == "":
            raise CustomException("not found any version")
        if self.__ver == self.__origin_issue.milestone.title:
            raise CustomException("backport version already exists in the currently issue.")
        pattern = re.compile(r"\[backport v\d+\.\d+\]")
        if re.match(pattern, self.__origin_issue.title):
            raise CustomException("it's not allowed to backport the backported issue.")

    def create_issue_if_not_exist(self):
        title = "[backport %s] %s" % (self.__ver[0:self.__ver.rindex('.')], self.__origin_issue.title)
        body = "backport the issue #%s" % self.__origin_issue.number

        # return if the comment exists
        comment_pattern = r'added `%s` issue: #[\d].' % self.__backport_needed.name
        comments = self.__origin_issue.get_comments()
        for comment in comments:
            if re.match(comment_pattern, comment.body):
                raise ExistedBackportComment("exists backport comment with %s" % self.__ver)
            
        issue_data = {
            'title': title,
            'body': body,
            'labels': self.__labels,
            'assignees': self.__origin_issue.assignees
        }
        
        if self.__milestone is not None:
            issue_data['milestone'] = self.__milestone
        
        self.__issue = repo.create_issue(**issue_data)

    def create_comment(self):
        self.__origin_issue.create_comment(
            body='added `%s` issue: #%d.' % (self.__backport_needed.name, self.__issue.number))