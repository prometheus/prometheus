set :base_url, "https://www.serf.io/"

activate :hashicorp do |h|
  h.name        = "serf"
  h.version     = "0.8.2"
  h.github_slug = "hashicorp/serf"
end

helpers do
  # Returns the FQDN of the image URL.
  #
  # @param [String] path
  #
  # @return [String]
  def image_url(path)
    File.join(base_url, image_path(path))
  end

  # Get the title for the page.
  #
  # @param [Middleman::Page] page
  #
  # @return [String]
  def title_for(page)
    if page && page.data.page_title
      return "#{page.data.page_title} - Serf by HashiCorp"
    end

     "Serf by HashiCorp"
   end

  # Get the description for the page
  #
  # @param [Middleman::Page] page
  #
  # @return [String]
  def description_for(page)
    description = (page.data.description || "")
      .gsub('"', '')
      .gsub(/\n+/, ' ')
      .squeeze(' ')

    return escape_html(description)
  end

  # This helps by setting the "active" class for sidebar nav elements
  # if the YAML frontmatter matches the expected value.
  def sidebar_current(expected)
    current = current_page.data.sidebar_current || ""
    if current.start_with?(expected)
      return " class=\"active\""
    else
      return ""
    end
  end

  # Returns the id for this page.
  # @return [String]
  def body_id_for(page)
    if !(name = page.data.sidebar_current).blank?
      return "page-#{name.strip}"
    end
    if page.url == "/" || page.url == "/index.html"
      return "page-home"
    end
    if !(title = page.data.page_title).blank?
      return title
        .downcase
        .gsub('"', '')
        .gsub(/[^\w]+/, '-')
        .gsub(/_+/, '-')
        .squeeze('-')
        .squeeze(' ')
    end
    return ""
  end

  # Returns the list of classes for this page.
  # @return [String]
  def body_classes_for(page)
    classes = []

    if !(layout = page.data.layout).blank?
      classes << "layout-#{page.data.layout}"
    end

    if !(title = page.data.page_title).blank?
      title = title
        .downcase
        .gsub('"', '')
        .gsub(/[^\w]+/, '-')
        .gsub(/_+/, '-')
        .squeeze('-')
        .squeeze(' ')
      classes << "page-#{title}"
    end

    return classes.join(" ")
  end
end
