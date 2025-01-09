LOG = None

def lookup(obj, *keys):
    """This function returns a value from a nested json object"""
    curr = obj
    for key in keys:
        if isinstance(curr, dict) and key in curr:
            curr = curr[key]
        else:
            return None
    return curr


def insert(obj, *path_and_value):
    """"This function inserts a value into a nested json object"""
    if len(path_and_value) < 2:
        raise ValueError("Error: insert() takes at least two args")

    *path, value = path_and_value

    curr = obj
    for key in path[:-1]:
        if key not in curr or not isinstance(curr[key], dict):
            curr[key] = {}
        curr = curr[key]

    curr[path[-1]] = value
