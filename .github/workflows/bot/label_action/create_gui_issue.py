from bot import repo
from bot.action import LabelAction
import re

CREATE_GUI_ISSUE_LABEL = "require/ui"
AREA_UI_LABEL = "area/ui"
class CreateGUIIssue(LabelAction):
    def __init__(self):
        pass
    
    def isMatched(self, request):
        matched = False
        
        if "backport" in request['issue']['title']:
            return False
        
        # We can't expect the labels order from Github Webhook request.
        # It might be ["require/ui", "area/ui"] or ["area/ui", "require/ui"]
        # So we need to consider those two cases.
        # "area/ui" is high priority label.
        # If "area/ui" is in the labels, we can skip the rest of the labels.
        for label in request['issue']['labels']:
            if AREA_UI_LABEL in label['name']:
                return False
            if CREATE_GUI_ISSUE_LABEL in label['name']:
                matched = True
            
        return matched
            
    def action(self, request):
        self.labels = [AREA_UI_LABEL]
        
        for label in request['issue']['labels']:
            if CREATE_GUI_ISSUE_LABEL not in label['name']:
                self.labels.append(label['name'])
    
        self.issue_number = request['issue']['number']
        self.original_issue = repo.get_issue(self.issue_number)
        
        if self.__is_gui_issue_exist():
            return "GUI issue already exist"
        self.__create_gui_issue()
        self.__create_comment()
        return "create GUI issue success"
    
    def __create_gui_issue(self):
        issue_data = {
            'title': f"[GUI] {self.original_issue.title}",
            'body': f"GUI Issue from #{self.issue_number}",
            'labels': self.labels,
        }
        if self.original_issue.milestone is not None:
            issue_data['milestone'] = self.original_issue.milestone
        self.gui_issue = repo.create_issue(**issue_data)
        
    def __create_comment(self):
        self.original_issue.create_comment(body=f"GUI issue created #{self.gui_issue.number}.")
        
    def __is_gui_issue_exist(self):
        comment_pattern = r'GUI issue created #[\d].'
        comments = self.original_issue.get_comments()
        for comment in comments:
            if re.match(comment_pattern, comment.body):
                return True
        return False