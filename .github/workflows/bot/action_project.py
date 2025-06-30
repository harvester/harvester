import logging
from bot.issue_transfer import IssueTransfer
from bot.action import Action
from bot import gtihub_project_manager, E2E_PIPELINE

class ActionProject(Action):
    def __init__(self):
        self.__event_type_key = "projects_v2_item"
    
    def isMatched(self, actionRequest):
        if actionRequest.event_type not in [self.__event_type_key]:
            return False
        if actionRequest.action not in ['edited']:
            return False
        return True
    
    def action(self, request):
        if request[self.__event_type_key]["content_type"] != "Issue":
            return
        
        project_node_id = request[self.__event_type_key]['project_node_id']
        if gtihub_project_manager.project()["id"] != project_node_id:
            logging.error("project is not matched")
            return
    
        # In github projectv2, every status filed changed will trigger a projectv2 event
        # For example, changing a `Estimate` and `Status`.
        # But, we only care about the `Status` field.
        if request['changes']['field_value']['field_name'] != "Status":
            return
        
        target_column = request['changes']['field_value']['to']
        if target_column["name"] not in E2E_PIPELINE.split(","):
            logging.info('target_column is {}, ignoring'.format(target_column["name"]))
            return
        
        issue_node_id = request[self.__event_type_key]['content_node_id']
        issue = gtihub_project_manager.get_global_issue(issue_node_id)
        
        if issue["number"] is None:
            logging.error("issue number is None")
            return
        
        it = IssueTransfer(issue["number"])
        it.create_comment_if_not_exist()