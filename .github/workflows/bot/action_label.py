import time
import random
from bot.label_action.create_gui_issue import CreateGUIIssue
from bot.label_action.create_backport import CreateBackport
from bot.label_action.create_e2e import CreateE2EIssue
from bot.action import Action

ALL_LABEL_ACTIONS = [
    CreateBackport,
    CreateGUIIssue,
    CreateE2EIssue
]

class ActionLabel(Action):
    def __init__(self):
        pass
    
    def isMatched(self, actionRequest):
        if actionRequest.event_type not in ['issue']:
            return False
        if actionRequest.action not in ['labeled']:
            return False
        return True
    
    def action(self, request):
        matched = False
        
        for label_action in ALL_LABEL_ACTIONS:
            __label_action = label_action()
            if __label_action.isMatched(request):
                matched = True
                time.sleep(1 + random.uniform(0.5, 1.5)) # jitter 
                __label_action.action(request)

        if not matched:
            return "No label action matched"
        
        return "labeled related actions succeed"

