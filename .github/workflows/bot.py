import sys
import json
import logging
from bot.action import ActionRequest
from bot.action_label import ActionLabel
from bot.action_project import ActionProject


SUPPORTED_ACTIONS = [
    ActionLabel(),
    ActionProject(),
]

SUPPORTED_EVENT = [
    "projects_v2_item",
    "issue"
]

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(message)s",
    datefmt="%Y-%m-%d %H:%M:%S", 
)

if __name__ == "__main__":
    request = sys.argv[1]
    
    try:
        req = json.loads(request)
    except json.JSONDecodeError as e:
        logging.error("Failed to parse JSON:", e)
        sys.exit(1)
    
    for event in SUPPORTED_EVENT:
        if req.get(event) is not None:
            event_type = event
    
    if event_type == "":
        sys.exit(0)
    
    action_request = ActionRequest(req.get('action'), event_type)
    matched = False
    
    for action in SUPPORTED_ACTIONS:
        if action.isMatched(action_request):
            matched = True
            msg = action.action(req)
            break
        
    logging.info(msg)
    
    if not matched:
        logging.info("No action matched")
        sys.exit(0)

    sys.exit(0)