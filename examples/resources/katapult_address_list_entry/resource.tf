# Address List to add Entries to 
resource "katapult_address_list" "web-1" {
    name = "web-1"
}

# Add an Entry to the Address List
resource "katapult_address_list_entry" "goog" {
    address_list_id = katapult_address_list.web-1.id
    address = "8.8.8.8"
    name = "Google DNS"
}